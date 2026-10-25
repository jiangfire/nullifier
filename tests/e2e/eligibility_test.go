package e2e

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"anonvote/internal/config"
	domaincrypto "anonvote/internal/domain/crypto"
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

// 资格证明的端到端用例 —— 路线图 T22 的验收 ①–⑤。
//
// 这里的服务器**只**拿到名单的根（root）与资格电路的验证密钥：它能验证
// 「这个人的资格在树里」，却无法把资格对应到任何成员。

// eligibilityFixture 是一次资格校验测试需要的全部东西。
type eligibilityFixture struct {
	// Server 是被测服务端。
	Server *httptest.Server
	// Recorded 是服务端收到的请求体（用来断言「没有真人身份字段」）。
	Recorded *[]recordedRequest
	// Root 是名单根。
	Root string
	// Members 是各成员的资格包。
	Members []eligibility.MemberPackage
	// Prover 是客户端侧的证明器（持有 proving key）。
	Prover *eligibility.Prover
}

// newEligibilityServer 起一个开启了资格校验的真实服务端。
//
// 名单用真实的 Merkle 树（不走 mock）：证明的验证必须是真的。
func newEligibilityServer(t *testing.T, adminToken string, memberCount int) *eligibilityFixture {
	t.Helper()

	// 资格电路的密钥。proving key 只在客户端侧（这里是测试进程），服务端
	// 只拿验证密钥。
	provingKey, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	keyDir := t.TempDir()
	verifyingKeyPath := filepath.Join(keyDir, "eligibility-vk.bin")
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o600))

	root, members := buildMemberPackages(t, memberCount)

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
	claimRepo := persistence.NewCredentialClaimRepository(db)
	eligibilityUseRepo := persistence.NewEligibilityUseRepository(db)

	proofVerifier, err := crypto.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := crypto.NewGnarkMerkleRootBuilder()
	logger := zap.NewNop()

	eligibilityService, err := crypto.NewEligibilityService(root, verifyingKeyPath)
	require.NoError(t, err)
	require.True(t, eligibilityService.Enabled())

	votingUC := usecase.NewVotingUseCase(
		voteRepo, participationRepo, credentialRepo, proposalRepo, sessionRepo,
		proofVerifier, encrypter, issuer, logger,
	)
	registerUC := usecase.NewRegisterUseCaseWithEligibility(
		credentialRepo, proposalRepo, issuer, claimRepo, eligibilityUseRepo,
		eligibilityService, logger,
	)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		encrypter, merkleBuilder, logger,
	)

	handler := httpapi.NewHandlerWithTally(
		votingUC, registerUC, proposalUC, participationUC, tallyUC, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)

	recorded := &[]recordedRequest{}
	engine := server.Engine()
	ts := httptest.NewServer(recordBodies(engine, recorded))
	t.Cleanup(ts.Close)

	prover, err := eligibility.NewProverFromBytes(provingKey)
	require.NoError(t, err)

	return &eligibilityFixture{
		Server:   ts,
		Recorded: recorded,
		Root:     root,
		Members:  members,
		Prover:   prover,
	}
}

// recordBodies 包一层，把每个请求体记录下来。
//
// 记录请求体是为了断言「服务端收到的领取请求里没有任何真人身份字段」。
func recordBodies(engine http.Handler, recorded *[]recordedRequest) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		*recorded = append(*recorded, recordedRequest{Path: r.URL.Path, Body: body})
		engine.ServeHTTP(w, r)
	})
}

// buildMemberPackages 造一份真实的资格名单。
//
// 叶子 = MiMC(秘密)，树按成员下标排列；每个成员拿到「秘密 + 叶子 + 路径」。
func buildMemberPackages(t *testing.T, count int) (string, []eligibility.MemberPackage) {
	t.Helper()

	secrets := make([][]byte, count)
	leaves := make([]fr.Element, count)
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

	rootElement, paths, err := eligibility.BuildTree(leaves, eligibility.TreeDepth)
	require.NoError(t, err)
	root := eligibility.EncodeField(rootElement)

	members := make([]eligibility.MemberPackage, 0, count)
	for i := range leaves {
		path := make([]string, len(paths[i]))
		for j, element := range paths[i] {
			path[j] = eligibility.EncodeField(element)
		}
		members = append(members, eligibility.MemberPackage{
			MemberID: fmt.Sprintf("member-%d", i),
			Secret:   hex.EncodeToString(secrets[i]),
			Leaf:     eligibility.EncodeField(leaves[i]),
			Index:    uint64(i),
			Path:     path,
			Root:     root,
			Depth:    eligibility.TreeDepth,
		})
	}
	return root, members
}

// eligibilityChallenge 取一次领取挑战。
func eligibilityChallenge(t *testing.T, fixture *eligibilityFixture, proposalID string) string {
	t.Helper()
	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/eligibility-challenge", map[string]any{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var payload struct {
		Root      string `json:"root"`
		Depth     int    `json:"depth"`
		Challenge string `json:"challenge"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&payload))
	require.Equal(t, fixture.Root, payload.Root, "服务端只认自己配置的那个根")
	require.Equal(t, eligibility.TreeDepth, payload.Depth)
	require.NotEmpty(t, payload.Challenge)
	return payload.Challenge
}

// eligibilityMaterial 用某个成员的资格包出一份证明材料。
func eligibilityMaterial(
	t *testing.T,
	fixture *eligibilityFixture,
	member eligibility.MemberPackage,
	proposalID string,
	challenge string,
) map[string]any {
	t.Helper()
	result, err := fixture.Prover.ProveEligibility(eligibility.ClientEligibilityRequest{
		MemberPackage: member,
		ProposalID:    proposalID,
		Challenge:     challenge,
		Root:          fixture.Root,
	})
	require.NoError(t, err)
	return map[string]any{
		"eligibility_proof": base64.StdEncoding.EncodeToString(result.Proof),
		"qualification_id":  result.QualificationID,
		"challenge":         challenge,
	}
}

// TestAPI_EligibilityRequiredForClaim_E2E 是验收 ①：
// 没有资格证明的领取被拒，而且**池槽位不被消耗**。
func TestAPI_EligibilityRequiredForClaim_E2E(t *testing.T) {
	const adminToken = "secret"
	fixture := newEligibilityServer(t, adminToken, 2)
	proposalID := createProposal(t, fixture.Server, adminToken)

	client := testutil.NewBlindClient(fixture.Server.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	// 空请求体：没带资格材料。
	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", map[string]any{})
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)

	var failure struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
	require.Equal(t, "ELIGIBILITY_REQUIRED", failure.Code)

	// 槽位没被消耗：合格的人仍然领得到第一个槽位。
	challenge := eligibilityChallenge(t, fixture, proposalID)
	material := eligibilityMaterial(t, fixture, fixture.Members[0], proposalID, challenge)
	resp = postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", material)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)
}

// TestAPI_EligibilityClaimSucceeds_E2E 是验收 ②：
// 有效资格证明能领到凭证（走完盲签名两步）。
func TestAPI_EligibilityClaimSucceeds_E2E(t *testing.T) {
	const adminToken = "secret"
	fixture := newEligibilityServer(t, adminToken, 2)
	proposalID := createProposal(t, fixture.Server, adminToken)

	client := testutil.NewBlindClient(fixture.Server.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	challenge := eligibilityChallenge(t, fixture, proposalID)
	material := eligibilityMaterial(t, fixture, fixture.Members[0], proposalID, challenge)

	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", material)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var reservation struct {
		UserID     string `json:"user_id"`
		Modulus    string `json:"modulus"`
		Exponent   int    `json:"exponent"`
		Variant    string `json:"variant"`
		KeyVersion int    `json:"issuer_key_version"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&reservation))
	require.NotEmpty(t, reservation.UserID)

	// 走完剩下两步：本地盲化 → 盲签名 → 解盲 → attest。
	modulus, err := base64.StdEncoding.DecodeString(reservation.Modulus)
	require.NoError(t, err)
	issuerKey := domainIssuerKey(
		reservation.KeyVersion, modulus, reservation.Exponent, reservation.Variant,
	)
	blindClient, err := crypto.NewCredentialBlindClient(&issuerKey)
	require.NoError(t, err)
	state, err := blindClient.Blind(reservation.UserID, []byte("one-time-public-key"))
	require.NoError(t, err)

	blindSignature, err := client.BlindSignPool(
		proposalID,
		&testutil.Reservation{
			UserID:           reservation.UserID,
			IssuerKeyVersion: reservation.KeyVersion,
		},
		state.BlindedMessage,
	)
	require.NoError(t, err)

	signature, err := blindClient.Finalize(state, blindSignature)
	require.NoError(t, err)

	credential, err := client.Attest(
		proposalID, reservation.UserID, []byte("one-time-public-key"), signature,
	)
	require.NoError(t, err)
	require.NotEmpty(t, credential.Signature)
}

// TestAPI_EligibilityCannotBeReused_E2E 是验收 ③：
// 同一份资格第二次领取被拒（唯一索引）。
func TestAPI_EligibilityCannotBeReused_E2E(t *testing.T) {
	const adminToken = "secret"
	fixture := newEligibilityServer(t, adminToken, 2)
	proposalID := createProposal(t, fixture.Server, adminToken)

	client := testutil.NewBlindClient(fixture.Server.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	challenge := eligibilityChallenge(t, fixture, proposalID)
	material := eligibilityMaterial(t, fixture, fixture.Members[0], proposalID, challenge)

	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", material)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	// 第二次：同一份资格（连挑战都一样）—— 撞唯一索引。
	resp = postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", material)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusConflict, resp.StatusCode)

	var failure struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
	require.Equal(t, "ELIGIBILITY_ALREADY_USED", failure.Code)
}

// TestAPI_EligibilityNotReusableAcrossProposals_E2E 是验收 ④：
// 提案 A 的证明拿去领提案 B —— 验签失败（提案 ID 在公开输入里）。
func TestAPI_EligibilityNotReusableAcrossProposals_E2E(t *testing.T) {
	const adminToken = "secret"
	fixture := newEligibilityServer(t, adminToken, 2)
	proposalA := createProposal(t, fixture.Server, adminToken)
	proposalB := createProposal(t, fixture.Server, adminToken)

	client := testutil.NewBlindClient(fixture.Server.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalA, 1))
	require.NoError(t, client.FillCredentialPool(proposalB, 1))

	// 证明是按提案 A 出的。
	challengeA := eligibilityChallenge(t, fixture, proposalA)
	materialA := eligibilityMaterial(t, fixture, fixture.Members[0], proposalA, challengeA)

	// 拿去领提案 B：证明里的提案 ID 对不上，验签必然失败。
	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalB+"/credentials/claim", materialA)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusForbidden, resp.StatusCode)

	var failure struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&failure))
	require.Equal(t, "INVALID_ELIGIBILITY_PROOF", failure.Code)
}

// TestAPI_EligibilityClaimBodyHasNoIdentityFields_E2E 是验收 ⑤：
// 领取请求体里不含任何真人身份字段。
func TestAPI_EligibilityClaimBodyHasNoIdentityFields_E2E(t *testing.T) {
	const adminToken = "secret"
	fixture := newEligibilityServer(t, adminToken, 2)
	proposalID := createProposal(t, fixture.Server, adminToken)

	client := testutil.NewBlindClient(fixture.Server.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	challenge := eligibilityChallenge(t, fixture, proposalID)
	material := eligibilityMaterial(t, fixture, fixture.Members[0], proposalID, challenge)
	resp := postJSON(t, fixture.Server.URL+"/api/v1/proposals/"+proposalID+"/credentials/claim", material)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var claimBody map[string]any
	for _, item := range *fixture.Recorded {
		if item.Path == "/api/v1/proposals/"+proposalID+"/credentials/claim" {
			claimBody = item.Body
		}
	}
	require.NotNil(t, claimBody, "必须走到领取这一步")

	keys := make([]string, 0, len(claimBody))
	for key := range claimBody {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	require.Equal(t, []string{"challenge", "eligibility_proof", "qualification_id"}, keys,
		"领取请求体里只允许出现这三样资格材料")

	// 逐个钉住「真人身份字段」的黑名单。
	forbidden := []string{
		"member_id", "employee_id", "id_card", "phone", "email", "name",
		"user_id", "user_pub_key", "voter_id", "identity",
	}
	for _, field := range forbidden {
		require.NotContains(t, claimBody, field, "领取请求体不许出现真人身份字段")
	}
}

// domainIssuerKey 把预留响应里的公钥字段还原成领域类型。
func domainIssuerKey(
	version int,
	modulus []byte,
	exponent int,
	variant string,
) domaincrypto.IssuerPublicKey {
	return domaincrypto.IssuerPublicKey{
		Version:  version,
		Modulus:  modulus,
		Exponent: exponent,
		Variant:  variant,
	}
}
