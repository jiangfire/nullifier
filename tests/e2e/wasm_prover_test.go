package e2e

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// wasmDriverScript 是驱动 WASM 模块的 node 脚本。
//
// 它复刻浏览器端的加载方式：用 Go 官方胶水 `wasm_exec.js` 起运行时，
// 拿到模块注册的 `nullifierProver` 全局对象，再调用 init / prove。
// 浏览器与 node 的差别只有「胶水来源」与「文件读写」，密码学路径完全一致。
const wasmDriverScript = `
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

  const initResult = api.init(
    JSON.stringify({ proving_key: fs.readFileSync(provingKeyPath).toString('base64') }),
  );
  if (!initResult.ok) {
    throw new Error('init failed: ' + initResult.error);
  }
  const proveResult = api.prove(fs.readFileSync(requestPath, 'utf8'));
  fs.writeFileSync(outputPath, JSON.stringify({ init: initResult, prove: proveResult }));
  process.exit(0);
})().catch((error) => {
  console.error(error && error.stack ? error.stack : String(error));
  process.exit(1);
});
`

// wasmProofOutput 是 WASM 模块跑完后的输出。
type wasmProofOutput struct {
	Init struct {
		OK     bool   `json:"ok"`
		Error  string `json:"error"`
		InitMS int64  `json:"init_ms"`
	} `json:"init"`
	Prove struct {
		OK         bool   `json:"ok"`
		Error      string `json:"error"`
		Ciphertext string `json:"ciphertext"`
		Nullifier  string `json:"nullifier"`
		Proof      string `json:"proof"`
		ProveMS    int64  `json:"prove_ms"`
	} `json:"prove"`
}

// TestAPI_WASMProver_E2E 是「浏览器端 prover」的端到端证据。
//
// 这条用例跑的是**门户将要加载的那个 WASM 模块**（GOOS=js GOARCH=wasm 构建），
// 只是把浏览器换成了 node：模块由 node 加载官方 wasm_exec.js 胶水，
// 拿到 proving key 后在本地生成密文、nullifier 与 Groth16 证明。
//
// 服务端配**真的 verifying key**，全程没有参与出证明，也从未看到明文选项：
// 请求里只有密文、nullifier 与证明。计票阶段用门限私钥还原出的选项，
// 必须与客户端选的那个一致 —— 这同时验证了「客户端拼的公开输入」与
// 「服务端自拼的公开输入」逐字段相同。
//
// 缺少 node 或 Go 官方胶水时跳过：浏览器端 prover 的产物仍可用
// `make web-wasm` 手工验收。
func TestAPI_WASMProver_E2E(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node 不在 PATH 上，跳过 WASM prover 验收")
	}
	glue := filepath.Join(runtime.GOROOT(), "lib", "wasm", "wasm_exec.js")
	if _, err := os.Stat(glue); err != nil {
		t.Skip("找不到 Go 官方 wasm_exec.js 胶水，跳过 WASM prover 验收")
	}

	const adminToken = "secret"

	// 电路密钥：proving key 在客户端侧（WASM 模块），verifying key 部署到服务端。
	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	provingKeyPath := filepath.Join(keyDir, "proving.key")
	verifyingKeyPath := filepath.Join(keyDir, "verifying.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o644))

	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	resultRepo := persistence.NewTallyResultRepository(db)
	anchorRepo := persistence.NewAuditAnchorRepository(db)

	proofVerifier, err := crypto.NewGnarkProofVerifier(verifyingKeyPath)
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := crypto.NewGnarkMerkleRootBuilder()

	logger := zap.NewNop()
	votingUC := usecase.NewVotingUseCase(
		voteRepo, participationRepo, credentialRepo, proposalRepo, sessionRepo,
		proofVerifier, encrypter, issuer, logger,
	)
	registerUC := usecase.NewRegisterUseCase(credentialRepo, proposalRepo, issuer, logger)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		encrypter, crypto.NewBallotNullifierGenerator(), merkleBuilder, logger,
	)

	// 非 debug 模式：服务端代加密的 ballot-draft 不可达。
	handler := httpapi.NewHandlerWithTallyAndMode(
		votingUC, registerUC, proposalUC, participationUC, tallyUC, false, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)
	ts := httptest.NewServer(server.Engine())
	defer ts.Close()

	// 1) 创建提案（两个选项）。
	resp := postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, map[string]any{
		"title":     "WASM Prover Election",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposalResp struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposalResp))
	resp.Body.Close()

	// 2) 签发凭证（真两段式盲签名）。
	credential := issueCredential(
		t, ts, adminToken, proposalResp.ID, "u-wasm", []byte("user-pub-key"),
	)

	// 3) 初始化门限解密会话。
	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/decryption/setup",
		adminToken, map[string]any{"threshold": 2, "trustees": 3})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var setupResp struct {
		PublicKey string   `json:"public_key"`
		Shares    []string `json:"shares"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setupResp))
	resp.Body.Close()
	require.Len(t, setupResp.Shares, 3)

	// 4) 门户侧要拿的东西：计票公钥（公开值）。
	resp, err = http.Get(ts.URL + "/api/v1/proposals/" + proposalResp.ID + "/election-key")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	var keyResp struct {
		PublicKey string `json:"public_key"`
		Encoding  string `json:"encoding"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&keyResp))
	resp.Body.Close()
	require.Equal(t, "uncompressed", keyResp.Encoding)

	// 5) 编译 WASM 模块，用 node 驱动它出证明。
	wasmPath := buildWASMProver(t)
	request := map[string]any{
		"proposal_id":         proposalResp.ID,
		"election_public_key": keyResp.PublicKey,
		"option_index":        1,
		"num_options":         2,
	}
	output := runWASMProver(t, node, glue, wasmPath, provingKeyPath, request)

	require.True(t, output.Init.OK, output.Init.Error)
	require.True(t, output.Prove.OK, output.Prove.Error)
	require.NotEmpty(t, output.Prove.Proof)

	ciphertext, err := base64.StdEncoding.DecodeString(output.Prove.Ciphertext)
	require.NoError(t, err)
	proof, err := base64.StdEncoding.DecodeString(output.Prove.Proof)
	require.NoError(t, err)
	nullifierBytes, err := hex.DecodeString(output.Prove.Nullifier)
	require.NoError(t, err)
	require.Len(t, nullifierBytes, ballot.ScalarSize)
	t.Logf(
		"WASM prover: init %dms, prove %dms, proof %d bytes",
		output.Init.InitMS, output.Prove.ProveMS, len(proof),
	)

	// 6) 先投一份**被改过**的证明：服务端必须真的在验签。
	tampered := append([]byte(nil), proof...)
	tampered[0] ^= 0x01
	resp = postJSON(t, ts.URL+"/api/v1/votes", map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(tampered),
		"nullifier":            output.Prove.Nullifier,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	resp.Body.Close()
	stored, err := voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Empty(t, stored, "验签失败的票不能落库")

	// 7) 提交 WASM 模块出的证明。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	resp = postJSON(t, ts.URL+"/api/v1/votes", map[string]any{
		"proposal_id":          proposalResp.ID,
		"encrypted_vote":       base64.StdEncoding.EncodeToString(ciphertext),
		"proof":                base64.StdEncoding.EncodeToString(proof),
		"nullifier":            output.Prove.Nullifier,
		"user_id":              credential.UserID,
		"user_pub_key":         credential.UserPubKey,
		"credential_signature": credential.Signature,
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	resp.Body.Close()

	stored, err = voteRepo.GetByProposal(t.Context(), proposalResp.ID)
	require.NoError(t, err)
	require.Len(t, stored, 1)
	require.Equal(t, ciphertext, stored[0].Cipher)

	// 8) 计票：门限私钥还原出浏览器端选的那个选项。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalResp.ID).
		Updates(map[string]any{
			"status":  "closed",
			"ends_at": time.Now().UTC().Add(-1 * time.Second),
		}).Error)

	resp = postAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposalResp.ID+"/results/publish",
		adminToken, map[string]any{"shares": setupResp.Shares[:2]})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var publishResp struct {
		TotalVotes int            `json:"total_votes"`
		Tally      map[string]int `json:"tally"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&publishResp))
	resp.Body.Close()
	require.Equal(t, 1, publishResp.TotalVotes)
	require.Equal(t, 1, publishResp.Tally["candidate-B"])
	require.Equal(t, 0, publishResp.Tally["candidate-A"])
}

// buildWASMProver 编译浏览器端 prover 模块。
func buildWASMProver(t *testing.T) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "ballot-prover.wasm")
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", output, "./cmd/ballot-wasm")
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
	combined, err := cmd.CombinedOutput()
	require.NoError(t, err, string(combined))
	return output
}

// runWASMProver 用 node 驱动 WASM 模块，返回模块的输出。
func runWASMProver(
	t *testing.T,
	node string,
	glue string,
	wasmPath string,
	provingKeyPath string,
	request map[string]any,
) wasmProofOutput {
	t.Helper()
	dir := t.TempDir()
	driverPath := filepath.Join(dir, "driver.cjs")
	require.NoError(t, os.WriteFile(driverPath, []byte(wasmDriverScript), 0o600))
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
	var output wasmProofOutput
	require.NoError(t, json.Unmarshal(raw, &output))
	return output
}

// repoRoot 返回仓库根目录：测试文件在 tests/e2e 下，往上两级即是。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok)
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
