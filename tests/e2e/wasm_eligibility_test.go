package e2e

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// wasmEligibilityDriverScript 是驱动 WASM 模块「资格证明」那半边的 node 脚本。
//
// 它复刻浏览器端的加载方式：用 Go 官方胶水 `wasm_exec.js` 起运行时，拿到
// 模块注册的 `nullifierProver` 全局对象，再调用 initEligibility /
// proveEligibility。浏览器与 node 的差别只有「胶水来源」与「文件读写」。
const wasmEligibilityDriverScript = `
const fs = require('fs');
const path = require('path');

const goroot = process.env.GOROOT || '';
globalThis.require = require;
globalThis.fs = fs;
globalThis.path = path;
globalThis.TextEncoder = require('util').TextEncoder;
globalThis.TextDecoder = require('util').TextDecoder;
globalThis.performance = globalThis.performance || require('perf_hooks').performance;
globalThis.crypto = globalThis.crypto || require('crypto').webcrypto;

require(path.join(goroot, 'lib', 'wasm', 'wasm_exec.js'));

async function waitForApi() {
  for (let i = 0; i < 200; i += 1) {
    if (globalThis.nullifierProver) {
      return globalThis.nullifierProver;
    }
    await new Promise((resolve) => setTimeout(resolve, 20));
  }
  throw new Error('nullifierProver API was not registered');
}

(async () => {
  const [wasmPath, provingKeyPath, requestPath, outputPath] = process.argv.slice(2);
  const go = new Go();
  const bytes = fs.readFileSync(wasmPath);
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  go.run(instance);
  const api = await waitForApi();

  const initResult = api.initEligibility(
    JSON.stringify({ proving_key: fs.readFileSync(provingKeyPath).toString('base64') }),
  );
  if (!initResult.ok) {
    throw new Error('initEligibility failed: ' + initResult.error);
  }
  const proveResult = api.proveEligibility(fs.readFileSync(requestPath, 'utf8'));
  fs.writeFileSync(outputPath, JSON.stringify({ init: initResult, prove: proveResult }));
  process.exit(0);
})().catch((error) => {
  console.error(error && error.stack ? error.stack : String(error));
  process.exit(1);
});
`

// wasmEligibilityOutput 是 WASM 模块跑完后的输出。
type wasmEligibilityOutput struct {
	Init struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		InitMS int64  `json:"init_ms"`
	} `json:"init"`
	Prove struct {
		OK              bool   `json:"ok"`
		Error           string `json:"error"`
		Proof           string `json:"proof"`
		QualificationID string `json:"qualification_id"`
		ProveMS         int64  `json:"prove_ms"`
	} `json:"prove"`
}

// TestAPI_WASMEligibilityProver_E2E 是「浏览器端出资格证明」的端到端证据。
//
// 这条用例跑的是**门户将要加载的那个 WASM 模块**，只是把浏览器换成 node：
// 模块在本地读成员包与 proving key，出具一份零知识资格证明。服务端配**真的
// 资格验证密钥**去验这份证明 —— 验得过，说明浏览器端与 Go 侧的公开输入拼装
// 逐字段相同（跨语言问题只会在验签失败时暴露）。
//
// 缺 node 或 Go 官方胶水时跳过。
func TestAPI_WASMEligibilityProver_E2E(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 不在 PATH 上，跳过 WASM 资格 prover 验收")
	}
	glue := filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js")
	if _, err := os.Stat(glue); err != nil {
		t.Skip("找不到 Go 官方 wasm_exec.js 胶水，跳过 WASM 资格 prover 验收")
	}

	// 资格电路的密钥：proving key 在客户端侧（WASM 模块），验证密钥在服务端。
	provingKey, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	provingKeyPath := filepath.Join(keyDir, "eligibility-proving.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))

	// 一份真实的成员资格包。
	root, pack := buildWASMMemberPackage(t)

	// 编译 WASM 模块，用 node 驱动它出资格证明。
	wasmPath := buildWASMProver(t)
	const proposalID = "proposal-wasm-eligibility"
	const challenge = "00aabbccddeeff00112233445566778899aabbccddeeff001122334455667788"
	request := map[string]any{
		"proposal_id":    proposalID,
		"challenge":      challenge,
		"root":           root,
		"member_package": pack,
	}
	output := runWASMEligibilityProver(t, node, glue, wasmPath, provingKeyPath, request)

	require.True(t, output.Init.OK, output.Init.Error)
	require.True(t, output.Prove.OK, output.Prove.Error)
	require.NotEmpty(t, output.Prove.Proof)
	require.NotEmpty(t, output.Prove.QualificationID)
	t.Logf(
		"WASM 资格 prover: init %dms, prove %dms",
		output.Init.InitMS, output.Prove.ProveMS,
	)

	// 服务端侧：用真的验证密钥验这份证明。
	verifier, err := eligibility.NewVerifierFromBytes(verifyingKey)
	require.NoError(t, err)
	proof, err := base64.StdEncoding.DecodeString(output.Prove.Proof)
	require.NoError(t, err)
	statement, err := eligibility.BuildStatement(
		root, proposalID, challenge, output.Prove.QualificationID,
	)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(proof, statement))

	// 去重键与 Go 侧自己算出来的一致：跨语言拼装不能有偏差。
	secretBytes, err := hex.DecodeString(pack.Secret)
	require.NoError(t, err)
	leaf, err := eligibility.DeriveLeaf(secretBytes)
	require.NoError(t, err)
	expected := eligibility.DeriveQualificationID(
		leaf, ballot.DeriveProposalField(proposalID),
	)
	require.Equal(t, eligibility.EncodeField(expected), output.Prove.QualificationID)
}

// buildWASMMemberPackage 造一份真实的成员资格包，返回名单根与成员包。
func buildWASMMemberPackage(t *testing.T) (string, eligibility.MemberPackage) {
	t.Helper()

	raw := make([]byte, eligibility.FieldSize)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	secret, err := eligibility.CanonicalSecret(raw)
	require.NoError(t, err)

	leaf, err := eligibility.DeriveLeaf(secret)
	require.NoError(t, err)
	rootElement, paths, err := eligibility.BuildTree(
		[]fr.Element{leaf}, eligibility.TreeDepth,
	)
	require.NoError(t, err)
	root := eligibility.EncodeField(rootElement)

	path := make([]string, len(paths[0]))
	for i, element := range paths[0] {
		path[i] = eligibility.EncodeField(element)
	}

	return root, eligibility.MemberPackage{
		MemberID: "member-wasm",
		Secret:   hex.EncodeToString(secret),
		Leaf:     eligibility.EncodeField(leaf),
		Index:    0,
		Path:     path,
		Root:     root,
		Depth:    eligibility.TreeDepth,
	}
}

// runWASMEligibilityProver 用 node 驱动 WASM 模块的资格证明 API。
func runWASMEligibilityProver(
	t *testing.T,
	node string,
	glue string,
	wasmPath string,
	provingKeyPath string,
	request map[string]any,
) wasmEligibilityOutput {
	t.Helper()
	dir := t.TempDir()
	driverPath := filepath.Join(dir, "driver.cjs")
	require.NoError(t, os.WriteFile(driverPath, []byte(wasmEligibilityDriverScript), 0o600))
	requestPath := filepath.Join(dir, "request.json")
	requestBytes, err := json.Marshal(request)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(requestPath, requestBytes, 0o600))
	outputPath := filepath.Join(dir, "output.json")

	cmd := exec.Command(node, driverPath, wasmPath, provingKeyPath, requestPath, outputPath)
	cmd.Env = append(os.Environ(), "GOROOT="+runtime.GOROOT())
	combined, err := cmd.CombinedOutput()
	require.NoError(t, err, string(combined))

	raw, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	var output wasmEligibilityOutput
	require.NoError(t, json.Unmarshal(raw, &output))
	return output
}
