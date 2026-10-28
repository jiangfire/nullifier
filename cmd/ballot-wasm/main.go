//go:build js && wasm

// Command ballot-wasm 是浏览器端的 prover。
//
// 它是 `cmd/ballot-wasm` 的 GOOS=js GOARCH=wasm 构建产物（见 `make web-wasm`），
// 在浏览器里被 Web Worker 加载，向 JS 暴露四个函数：
//
//	init(provingKeyBase64)            -> JSON 字符串   加载选票 proving key（约 7 秒）
//	prove(requestJSON)               -> JSON 字符串   生成密文、nullifier 与 Groth16 证明
//	initEligibility(provingKeyBase64)-> JSON 字符串   加载资格 proving key（约 5 秒）
//	proveEligibility(requestJSON)    -> JSON 字符串   生成资格证明与去重键
//
// 加密、nullifier 派生、公开输入拼装与证明全部在 Go 侧完成：门户只负责
// 取 proving key、传选项下标、拿结果提交。这样客户端与服务端共用同一套
// 代码（`internal/infrastructure/crypto/ballot` 与 `.../eligibility`），不会出现
// 「两侧公开输入拼得不一样导致证明验不过」的跨语言问题。
//
// 两套电路的密钥**互相独立**（Groth16 的 setup 是 per-circuit）：选票用
// `proving.key`，资格证明用 `eligibility-proving.key`。
//
// proving key 只在客户端侧使用，不能进服务端、不能入库。
package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark/logger"
)

// apiName 是暴露给 JS 的全局对象名。
const apiName = "nullifierProver"

// initRequest 是 init 的入参。
type initRequest struct {
	// ProvingKey 是 proving key 的 base64 编码。
	ProvingKey string `json:"proving_key"`
}

// proveRequest 是 prove 的入参。
type proveRequest struct {
	// ProposalID 是提案 ID 原文。
	ProposalID string `json:"proposal_id"`
	// ElectionPublicKey 是计票公钥的 base64 编码（未压缩，X 与 Y 各 32 字节大端）。
	ElectionPublicKey string `json:"election_public_key"`
	// OptionIndex 是投票人选中的选项下标。
	OptionIndex uint64 `json:"option_index"`
	// NumOptions 是提案的选项数。
	NumOptions int `json:"num_options"`
}

// eligibilityInitRequest 是 initEligibility 的入参。
type eligibilityInitRequest struct {
	// ProvingKey 是资格电路 proving key 的 base64 编码。
	ProvingKey string `json:"proving_key"`
	// Depth 是名单树深（可选）：proving key 是按深度生成的，成员包与
	// 密钥必须是同一深度。缺省（0）取默认深度 10 —— 存量密钥都是它。
	Depth int `json:"depth"`
}

// eligibilityProveRequest 是 proveEligibility 的入参。
type eligibilityProveRequest struct {
	// ProposalID 是提案 ID 原文。
	ProposalID string `json:"proposal_id"`
	// Challenge 是服务端签发的领取挑战（十六进制的域元素）。
	Challenge string `json:"challenge"`
	// Root 是服务端配置的名单树根（十六进制的域元素）。
	Root string `json:"root"`
	// MemberPackage 是成员自己的资格包（秘密 + 叶子 + Merkle 路径）。
	MemberPackage eligibility.MemberPackage `json:"member_package"`
}

// prover 是全局唯一的选票证明器：proving key 与编译好的电路只保留一份。
var prover *ballot.Prover

// eligibilityProver 是全局唯一的资格证明器。
var eligibilityProver *eligibility.Prover

// initFunc / proveFunc 必须留在 Go 侧：js.Func 被回收后对应的 JS 函数会失效。
var (
	initFunc             js.Func
	proveFunc            js.Func
	initEligibilityFunc  js.Func
	proveEligibilityFunc js.Func
)

func main() {
	// gnark 的日志必须关掉：js/wasm 往 stdout / stderr 写日志是「发请求给
	// JS 侧、再等回复」的同步调用，而 JS 侧此刻正卡在这次同步回调里，事件
	// 循环转不起来 —— 结果是整个运行时死锁（all goroutines are asleep）。
	logger.Disable()

	api := js.Global().Get("Object").New()
	initFunc = js.FuncOf(handleInit)
	proveFunc = js.FuncOf(handleProve)
	initEligibilityFunc = js.FuncOf(handleInitEligibility)
	proveEligibilityFunc = js.FuncOf(handleProveEligibility)
	api.Set("init", initFunc)
	api.Set("prove", proveFunc)
	api.Set("initEligibility", initEligibilityFunc)
	api.Set("proveEligibility", proveEligibilityFunc)
	js.Global().Set(apiName, api)
	// 阻塞住：Go 运行时必须一直在，JS 才能随时回调进来。
	select {}
}

// handleInit 加载 proving key 并编译电路。
//
// 出参是 JS 对象：{"ok":true,"init_ms":7354} 或 {"ok":false,"error":"..."}。
func handleInit(this js.Value, args []js.Value) (result any) {
	start := time.Now()
	response := map[string]any{"ok": false}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = map[string]any{"ok": false, "error": fmt.Sprintf("panic: %v", recovered)}
		}
		response["init_ms"] = time.Since(start).Milliseconds()
		result = response
	}()
	if len(args) != 1 {
		response["error"] = "init 需要一个参数：proving key 的 base64"
		return response
	}
	var request initRequest
	if err := json.Unmarshal([]byte(args[0].String()), &request); err != nil {
		response["error"] = fmt.Sprintf("解析 init 参数失败: %v", err)
		return response
	}
	raw, err := base64.StdEncoding.DecodeString(request.ProvingKey)
	if err != nil {
		response["error"] = fmt.Sprintf("proving key 不是合法的 base64: %v", err)
		return response
	}
	loaded, err := ballot.NewProverFromBytes(raw)
	if err != nil {
		response["error"] = fmt.Sprintf("加载 proving key 失败: %v", err)
		return response
	}
	prover = loaded
	response["ok"] = true
	return response
}

// handleProve 生成一张可提交的选票。
//
// 出参是 JS 对象：{"ok":true,"ciphertext":"...","nullifier":"...","proof":"...",
// "prove_ms":2180} 或 {"ok":false,"error":"..."}。
func handleProve(this js.Value, args []js.Value) (result any) {
	start := time.Now()
	response := map[string]any{"ok": false}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = map[string]any{"ok": false, "error": fmt.Sprintf("panic: %v", recovered)}
		}
		response["prove_ms"] = time.Since(start).Milliseconds()
		result = response
	}()
	if len(args) != 1 {
		response["error"] = "prove 需要一个参数：请求 JSON"
		return response
	}
	if prover == nil {
		response["error"] = "prover 尚未初始化：先调用 init"
		return response
	}
	var request proveRequest
	if err := json.Unmarshal([]byte(args[0].String()), &request); err != nil {
		response["error"] = fmt.Sprintf("解析 prove 参数失败: %v", err)
		return response
	}
	electionKey, err := base64.StdEncoding.DecodeString(request.ElectionPublicKey)
	if err != nil {
		response["error"] = fmt.Sprintf("计票公钥不是合法的 base64: %v", err)
		return response
	}
	ballotResult, err := prover.ProveBallot(ballot.ClientBallotRequest{
		ProposalID:        request.ProposalID,
		ElectionPublicKey: electionKey,
		OptionIndex:       request.OptionIndex,
		NumOptions:        request.NumOptions,
	})
	if err != nil {
		response["error"] = fmt.Sprintf("生成选票失败: %v", err)
		return response
	}
	response["ok"] = true
	response["ciphertext"] = base64.StdEncoding.EncodeToString(ballotResult.Ciphertext)
	response["nullifier"] = hex.EncodeToString(ballotResult.Nullifier)
	response["proof"] = base64.StdEncoding.EncodeToString(ballotResult.Proof)
	return response
}

// handleInitEligibility 加载资格电路的 proving key 并编译电路。
//
// 出参是 JS 对象：{"ok":true,"init_ms":5120} 或 {"ok":false,"error":"..."}。
func handleInitEligibility(this js.Value, args []js.Value) (result any) {
	start := time.Now()
	response := map[string]any{"ok": false}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = map[string]any{"ok": false, "error": fmt.Sprintf("panic: %v", recovered)}
		}
		response["init_ms"] = time.Since(start).Milliseconds()
		result = response
	}()
	if len(args) != 1 {
		response["error"] = "initEligibility 需要一个参数：proving key 的 base64"
		return response
	}
	var request eligibilityInitRequest
	if err := json.Unmarshal([]byte(args[0].String()), &request); err != nil {
		response["error"] = fmt.Sprintf("解析 initEligibility 参数失败: %v", err)
		return response
	}
	raw, err := base64.StdEncoding.DecodeString(request.ProvingKey)
	if err != nil {
		response["error"] = fmt.Sprintf("proving key 不是合法的 base64: %v", err)
		return response
	}
	// 深度缺省取默认值：存量 proving key 都是按默认深度生成的。
	depth := request.Depth
	if depth == 0 {
		depth = eligibility.DefaultTreeDepth
	}
	loaded, err := eligibility.NewProverFromBytesAtDepth(raw, depth)
	if err != nil {
		response["error"] = fmt.Sprintf("加载资格 proving key 失败: %v", err)
		return response
	}
	eligibilityProver = loaded
	response["ok"] = true
	return response
}

// handleProveEligibility 生成一份资格证明。
//
// 出参是 JS 对象：{"ok":true,"proof":"...","qualification_id":"...",
// "prove_ms":2180} 或 {"ok":false,"error":"..."}。
func handleProveEligibility(this js.Value, args []js.Value) (result any) {
	start := time.Now()
	response := map[string]any{"ok": false}
	defer func() {
		if recovered := recover(); recovered != nil {
			response = map[string]any{"ok": false, "error": fmt.Sprintf("panic: %v", recovered)}
		}
		response["prove_ms"] = time.Since(start).Milliseconds()
		result = response
	}()
	if len(args) != 1 {
		response["error"] = "proveEligibility 需要一个参数：请求 JSON"
		return response
	}
	if eligibilityProver == nil {
		response["error"] = "资格 prover 尚未初始化：先调用 initEligibility"
		return response
	}
	var request eligibilityProveRequest
	if err := json.Unmarshal([]byte(args[0].String()), &request); err != nil {
		response["error"] = fmt.Sprintf("解析 proveEligibility 参数失败: %v", err)
		return response
	}
	proof, err := eligibilityProver.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: request.MemberPackage,
		ProposalID:    request.ProposalID,
		Challenge:     request.Challenge,
		Root:          request.Root,
	})
	if err != nil {
		response["error"] = fmt.Sprintf("生成资格证明失败: %v", err)
		return response
	}
	response["ok"] = true
	response["proof"] = base64.StdEncoding.EncodeToString(proof.Proof)
	response["qualification_id"] = proof.QualificationID
	return response
}
