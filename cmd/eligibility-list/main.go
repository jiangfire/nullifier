// Command eligibility-list 由权威成员名单生成资格名单的 Merkle 承诺。
//
// 它是**离线工具**：不联网、不连服务端，跑在运营方（或独立的登记处）的机器上。
//
// 产物有两份，**必须分开保管**：
//
//	root.json            名单树根 —— 部署到投票服务端（crypto.eligibility_root）
//	eligibility-*.json   每位成员一份资格包（秘密 + 叶子 + 路径）—— 走安全渠道发给本人
//
// 服务端只拿到 root：它能验证「这个人的叶子在这棵名单树里」，却**无法把叶子
// 对应到任何成员**（它没有成员包，也没有名单）。这是能力 G（分发不泄露身份）
// 与「一人一票」能同时成立的关键。
//
// 用法：
//
//	go run ./cmd/eligibility-list -members members.txt -out ./eligibility-out
package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 解析参数并生成名单，返回进程退出码。
func run(args []string) int {
	fs := flag.NewFlagSet("nullifier-eligibility-list", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	membersPath := fs.String("members", "", "file with one member id per line")
	outDir := fs.String("out", "./eligibility-out", "directory to write the list root and member packages into")
	force := fs.Bool("force", false, "overwrite existing files")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, "用法: nullifier-eligibility-list -members <名单文件> [-out <目录>] [-force]")
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}
	if strings.TrimSpace(*membersPath) == "" {
		fmt.Fprintln(os.Stderr, "必须提供 -members")
		return 2
	}

	memberIDs, err := readMembers(*membersPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取名单失败: %v\n", err)
		return 1
	}
	if len(memberIDs) == 0 {
		fmt.Fprintln(os.Stderr, "名单是空的")
		return 1
	}

	if err := generateList(memberIDs, *outDir, *force); err != nil {
		fmt.Fprintf(os.Stderr, "生成名单失败: %v\n", err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "资格名单已生成: %d 名成员, 树深 %d\n", len(memberIDs), eligibility.TreeDepth)
	fmt.Fprintf(os.Stdout, "名单树根（部署到服务端 crypto.eligibility_root）: %s\n",
		filepath.Join(*outDir, "root.json"))
	fmt.Fprintf(os.Stdout, "成员资格包: %s/eligibility-<member_id>.json —— 逐个走安全渠道发给本人\n", *outDir)
	return 0
}

// readMembers 读名单文件：每行一个成员标识，空行与 # 开头的行忽略。
func readMembers(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open members file: %w", err)
	}
	defer func() { _ = file.Close() }()

	seen := map[string]struct{}{}
	members := make([]string, 0, 64)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, duplicated := seen[line]; duplicated {
			continue
		}
		seen[line] = struct{}{}
		members = append(members, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read members file: %w", err)
	}
	// 排序让成员的位置是**确定的**：同一份名单每次生成时成员的叶子位置一致。
	// 但资格秘密每次运行都会重新随机生成，所以**两次运行的树根不一样** ——
	// 名单与成员包必须成套保存，不能只留 root。
	sort.Strings(members)
	return members, nil
}

// generateList 生成树根与成员包并落盘。
func generateList(memberIDs []string, outDir string, force bool) error {
	secrets := make([][]byte, len(memberIDs))
	leaves := make([]fr.Element, len(memberIDs))
	for i := range memberIDs {
		secret := make([]byte, eligibility.FieldSize)
		if _, err := rand.Read(secret); err != nil {
			return fmt.Errorf("generate secret: %w", err)
		}
		canonical, err := eligibility.CanonicalSecret(secret)
		if err != nil {
			return fmt.Errorf("canonicalize secret: %w", err)
		}
		secrets[i] = canonical

		leaf, err := eligibility.DeriveLeaf(canonical)
		if err != nil {
			return fmt.Errorf("derive leaf: %w", err)
		}
		leaves[i] = leaf
	}

	root, paths, err := eligibility.BuildTree(leaves, eligibility.TreeDepth)
	if err != nil {
		return fmt.Errorf("build tree: %w", err)
	}
	rootHex := eligibility.EncodeField(root)

	if err := os.MkdirAll(outDir, 0o700); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	list := eligibility.ListPackage{
		Root:  rootHex,
		Depth: eligibility.TreeDepth,
		Count: len(memberIDs),
	}
	if err := writeJSON(filepath.Join(outDir, "root.json"), list, 0o644, force); err != nil {
		return err
	}

	for i, memberID := range memberIDs {
		path := make([]string, len(paths[i]))
		for j := range paths[i] {
			path[j] = eligibility.EncodeField(paths[i][j])
		}
		pack := eligibility.MemberPackage{
			MemberID: memberID,
			Secret:   hexString(secrets[i]),
			Leaf:     eligibility.EncodeField(leaves[i]),
			Index:    uint64(i),
			Path:     path,
			Root:     rootHex,
			Depth:    eligibility.TreeDepth,
		}
		name := fmt.Sprintf("eligibility-%s.json", sanitizeFileName(memberID))
		if err := writeJSON(filepath.Join(outDir, name), pack, 0o600, force); err != nil {
			return err
		}
	}
	return nil
}

// writeJSON 写 JSON 文件。
func writeJSON(path string, payload any, mode os.FileMode, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s already exists (use -force to overwrite)", path)
		}
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", path, err)
	}
	raw = append(raw, '\n')
	if err := os.WriteFile(path, raw, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// sanitizeFileName 把成员标识变成安全的文件名。
func sanitizeFileName(memberID string) string {
	var out strings.Builder
	for _, r := range memberID {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '_', r == '.':
			out.WriteRune(r)
		default:
			out.WriteRune('_')
		}
	}
	return out.String()
}

// hexString 把字节编成小写十六进制。
func hexString(raw []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, 0, len(raw)*2)
	for _, b := range raw {
		out = append(out, digits[b>>4], digits[b&0x0f])
	}
	return string(out)
}
