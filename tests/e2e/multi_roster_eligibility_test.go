package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/eligibility"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"
	"anonvote/tests/testutil"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// 多套资格名单并存的端到端用例（v0.4.0 · T30）。
//
// 一台服务端配两套名单（各自独立的根、验证密钥与树深），提案分别绑定：
// 每个提案只认自己绑定名单的证明，跨名单的成员包必然验不过 —— 而服务端
// 仍然只见各名单的根，看不到任何名单本身。

// rosterSet 是一套名单的测试材料。
type rosterSet struct {
	name    string
	depth   int
	root    string
	members []eligibility.MemberPackage
	prover  *eligibility.Prover
}

// buildRosterSetWithKeys 造一套指定树深与名字的名单（真实 Merkle 树 + 真实
// 密钥对），返回名单材料与**验证密钥的路径**（服务端要拿的那一半）。
func buildRosterSetWithKeys(t *testing.T, name string, depth int, memberCount int) (*rosterSet, string) {
	t.Helper()

	provingKey, verifyingKey, err := eligibility.GenerateKeys(depth)
	require.NoError(t, err)
	verifyingKeyPath := filepath.Join(t.TempDir(), "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o600))

	secrets := make([][]byte, memberCount)
	leaves := make([]fr.Element, memberCount)
	for i := range secrets {
		raw := make([]byte, eligibility.FieldSize)
		_, err := rand.Read(raw)
		require.NoError(t, err)
		canonical, err := eligibility.CanonicalSecret(raw)
		require.NoError(t, err)
		secrets[i] = canonical

		leaf, err := eligibility.DeriveLeaf(canonical)
		require.NoError(t, err)
		leaves[i] = leaf
	}
	rootElement, paths, err := eligibility.BuildTree(leaves, depth)
	require.NoError(t, err)
	root := eligibility.EncodeField(rootElement)

	members := make([]eligibility.MemberPackage, memberCount)
	for i := range members {
		path := make([]string, len(paths[i]))
		for j := range paths[i] {
			path[j] = eligibility.EncodeField(paths[i][j])
		}
		members[i] = eligibility.MemberPackage{
			MemberID: fmt.Sprintf("%s-member-%d", name, i),
			Secret:   hex.EncodeToString(secrets[i]),
			Leaf:     eligibility.EncodeField(leaves[i]),
			Index:    uint64(i),
			Path:     path,
			Root:     root,
			Depth:    depth,
		}
	}
	prover, err := eligibility.NewProverFromBytesAtDepth(provingKey, depth)
	require.NoError(t, err)

	return &rosterSet{
		name:    name,
		depth:   depth,
		root:    root,
		members: members,
		prover:  prover,
	}, verifyingKeyPath
}

// TestAPI_MultiRosterEligibilityScoped_E2E 是多名单的完整链路证据：
// 两套名单（各自密钥）、两个提案（各绑一套），逐条验证「各认各的、跨名单被拒、
// 默认名单兜底、未知名单创建时被拒」。
func TestAPI_MultiRosterEligibilityScoped_E2E(t *testing.T) {
	adminToken := "test-admin-token"

	// 两套名单：小名单刻意用非默认深度（顺带覆盖深度接线），另一套用默认深度。
	rosterA, vkA := buildRosterSetWithKeys(t, "acme-2026", eligibility.MinTreeDepth, 2)
	rosterB, vkB := buildRosterSetWithKeys(t, "default", eligibility.DefaultTreeDepth, 2)

	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: ":memory:"})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))

	voteRepo := persistence.NewVoteRepository(db)
	credentialRepo := persistence.NewCredentialRepository(db)
	proposalRepo := persistence.NewProposalRepository(db)
	participationRepo := persistence.NewParticipationRepository(db)
	sessionRepo := persistence.NewDecryptionSessionRepository(db)
	claimRepo := persistence.NewCredentialClaimRepository(db)
	eligibilityUseRepo := persistence.NewEligibilityUseRepository(db)

	proofVerifier, err := crypto.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	logger := zap.NewNop()

	// 服务端拿两套名单的**根 + 验证密钥 + 树深**，别的什么都没有。
	eligibilityService, err := crypto.NewEligibilityServiceFromSpecs([]crypto.RosterSpec{
		{Name: rosterA.name, Root: rosterA.root, VerifyingKeyPath: vkA, Depth: rosterA.depth},
		{Name: rosterB.name, Root: rosterB.root, VerifyingKeyPath: vkB, Depth: rosterB.depth},
	})
	require.NoError(t, err)
	require.True(t, eligibilityService.Enabled())

	registerUC := usecase.NewRegisterUseCaseWithEligibility(
		credentialRepo, proposalRepo, issuer, claimRepo, eligibilityUseRepo,
		eligibilityService, logger,
	)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, eligibilityService, logger)

	handler := httpapi.NewHandlerWithTally(
		usecase.NewVotingUseCase(voteRepo, participationRepo, credentialRepo, proposalRepo,
			sessionRepo, proofVerifier, encrypter, issuer, logger),
		registerUC, proposalUC,
		usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger),
		nil, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)
	ts := httptest.NewServer(server.Engine())
	t.Cleanup(ts.Close)

	// 两个提案：一个绑定 acme-2026，一个不绑定（走默认名单）。
	scopedProposal := createProposalForRoster(t, ts, adminToken, "scoped-proposal", rosterA.name)
	defaultProposal := createProposalForRoster(t, ts, adminToken, "default-proposal", "")
	require.NotEmpty(t, scopedProposal)
	require.NotEmpty(t, defaultProposal)

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(scopedProposal, 2))
	require.NoError(t, client.FillCredentialPool(defaultProposal, 2))

	// ① acme 成员在绑定了 acme 的提案上领取 → 成功。
	fixtureA := &eligibilityFixture{Server: ts, Root: rosterA.root, Members: rosterA.members, Prover: rosterA.prover, Depth: rosterA.depth}
	challengeA := eligibilityChallenge(t, fixtureA, scopedProposal)
	material := eligibilityMaterial(t, fixtureA, rosterA.members[0], scopedProposal, challengeA)
	resp := postJSON(t, ts.URL+"/api/v1/proposals/"+scopedProposal+"/credentials/claim", material)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "acme 成员在 acme 提案上应当领取成功")

	// ② 同一套 acme 的成员包投到默认名单的提案上 → 证明必败（名单不同）。
	fixtureB := &eligibilityFixture{Server: ts, Root: rosterB.root, Members: rosterB.members, Prover: rosterB.prover, Depth: rosterB.depth}
	challengeB := eligibilityChallenge(t, fixtureB, defaultProposal)
	materialWrong := eligibilityMaterial(t, fixtureA, rosterA.members[1], defaultProposal, challengeB)
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+defaultProposal+"/credentials/claim", materialWrong)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusForbidden, resp.StatusCode, "acme 成员包在默认名单的提案上必须被拒")

	// ③ default 名单的成员在不绑定的提案上领取 → 成功（默认名单兜底）。
	materialB := eligibilityMaterial(t, fixtureB, rosterB.members[0], defaultProposal, challengeB)
	resp = postJSON(t, ts.URL+"/api/v1/proposals/"+defaultProposal+"/credentials/claim", materialB)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "default 成员在不绑定的提案上应当领取成功")

	// ④ 创建提案时绑定不存在的名单域 → 400（创建时就拦住）。
	resp = adminPostJSON(t, ts, adminToken, "/api/v1/proposals", map[string]any{
		"title":             "bad-scope",
		"options":           []string{"a", "b"},
		"starts_at":         time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
		"ends_at":           time.Now().UTC().Add(2 * time.Hour).Format(time.RFC3339),
		"eligibility_scope": "nope-2026",
	})
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, "未知名单域必须在创建时被拒")
}

// adminPostJSON 带 admin 令牌 POST 一个 JSON 请求。
func adminPostJSON(t *testing.T, ts *httptest.Server, token string, path string, body any) *http.Response {
	t.Helper()
	raw, err := json.Marshal(body)
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, ts.URL+path, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := ts.Client().Do(req)
	require.NoError(t, err)
	return resp
}

// createProposalForRoster 创建一个提案，返回提案 ID；scope 为空表示不绑定。
func createProposalForRoster(t *testing.T, ts *httptest.Server, token string, title string, scope string) string {
	t.Helper()
	body := map[string]any{
		"title":     title,
		"options":   []string{"a", "b"},
		"starts_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}
	if scope != "" {
		body["eligibility_scope"] = scope
	}
	resp := adminPostJSON(t, ts, token, "/api/v1/proposals", body)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode, "创建提案应当成功")

	var created struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&created))
	return created.ID
}
