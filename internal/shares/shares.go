// Package shares 处理门限解密份额的读取与校验。
//
// 份额是**秘密材料**：拿到门限份数就能还原计票私钥、还原全部选票的明文选项。
// 所以这里的两件事都很敏感 —— 读取时不做任何日志输出，校验时只比对哈希。
//
// 为什么单独一个包：CLI（result-publish）与审计复算工具（audit-recompute）
// 都要吃同一批份额文件，格式有三种（JSON 数组 / 会话对象 / 纯文本行）。
// 两个 CLI 各写一份解析，格式迟早会漂移 —— 那时「发布结果用的份额文件」和
// 「审计复算用的份额文件」会变成两套东西。
package shares

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

// Hash 计算单个份额的哈希。
//
// 会话里只存份额的哈希，不存份额本身 —— 这样即使会话记录被导出，
// 也拿不到足以还原私钥的材料。
func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// HashList 计算一批份额的哈希。
func HashList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, Hash(value))
	}
	return out
}

// EnsureBelongToSession 校验提交的份额确实属于本会话。
//
// registered 是会话里登记的份额哈希列表，provided 是这次提交的份额。
// 两个约束：
//   - 每个提交的份额的哈希必须在登记列表里（防止拿别的会话的份额来凑）
//   - 同一个份额不能重复提交（重复的一份不能计入门限份数）
func EnsureBelongToSession(registered []string, provided []string) error {
	allowed := make(map[string]struct{}, len(registered))
	for _, item := range registered {
		allowed[item] = struct{}{}
	}
	seen := make(map[string]struct{}, len(provided))
	for _, item := range provided {
		digest := Hash(item)
		if _, ok := allowed[digest]; !ok {
			return errors.New("share does not belong to session")
		}
		// Repeating one share must not count towards the threshold.
		if _, ok := seen[digest]; ok {
			return errors.New("duplicate share submitted")
		}
		seen[digest] = struct{}{}
	}
	return nil
}

// EncodeList 把份额编码成 base64 字符串列表。
func EncodeList(values [][]byte) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, base64.StdEncoding.EncodeToString(value))
	}
	return out
}

// DecodeList 把 base64 编码的份额列表解码成字节。
func DecodeList(values []string) ([][]byte, error) {
	out := make([][]byte, 0, len(values))
	for _, item := range values {
		data, err := base64.StdEncoding.DecodeString(item)
		if err != nil {
			return nil, err
		}
		out = append(out, data)
	}
	return out, nil
}

// ParseRegistered 解析会话里登记的份额哈希列表。
//
// 会话表里存的是哈希（不是份额本身），所以这一步解析出来的每个元素
// 都已经是 64 位十六进制串。
func ParseRegistered(raw string) ([]string, error) {
	if raw == "" {
		return nil, errors.New("empty session shares")
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("no shares in session")
	}
	return out, nil
}

// SplitCSV 按逗号切分并去掉空项。
func SplitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, item := range parts {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// Load 读取份额：raw 是逗号分隔的份额列表，file 是份额文件路径。
//
// 文件支持三种形态，按顺序尝试：
//  1. JSON 数组：["份额1","份额2"]
//  2. 会话对象：{"shares":["份额1","份额2"]} —— `cli decrypt-setup -out` 写出来的就是它
//  3. 纯文本：每行一个份额（或用逗号/空格分隔）
//
// raw 与 file 至少给一个；raw 优先。
func Load(raw string, file string) ([]string, error) {
	if strings.TrimSpace(raw) != "" {
		values := SplitCSV(raw)
		if len(values) == 0 {
			return nil, errors.New("shares 为空")
		}
		return values, nil
	}
	if strings.TrimSpace(file) == "" {
		return nil, errors.New("必须提供 -shares 或 -shares-file")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	var list []string
	if err := json.Unmarshal(data, &list); err == nil && len(list) > 0 {
		return list, nil
	}
	// `decrypt-setup -out` 写出来的是整个会话对象（`{"shares":[...]}`）——
	// 这是运营方最自然的用法，发布结果时必须能直接吃这个文件。
	var session struct {
		Shares []string `json:"shares"`
	}
	if err := json.Unmarshal(data, &session); err == nil && len(session.Shares) > 0 {
		return session.Shares, nil
	}
	values := strings.FieldsFunc(string(data), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(values))
	for _, item := range values {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("shares 文件为空")
	}
	return out, nil
}
