package e2e

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"anonvote/internal/config"
	"anonvote/internal/infrastructure/crypto"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"
	"anonvote/internal/usecase"
	"anonvote/tests/testutil"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// recordedRequest 是一条被记录下来的请求。
type recordedRequest struct {
	Path string
	Body map[string]any
}

// interceptor 可以短路一次请求：返回 true 表示请求已经被处理掉了，
// 不会再走到真实的服务端。
type interceptor func(http.ResponseWriter, *http.Request) bool

// newBlindSignatureServer 起一个完整的真实服务端，并把请求体记录下来。
//
// 记录请求体是为了断言「服务端在签名阶段只见盲化消息」。
func newBlindSignatureServer(
	t *testing.T,
	adminToken string,
	interceptors ...interceptor,
) (*httptest.Server, *[]recordedRequest) {
	t.Helper()

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

	proofVerifier, err := crypto.NewGnarkProofVerifier("")
	require.NoError(t, err)
	encrypter := crypto.NewBallotThresholdCrypto()
	issuer := crypto.NewKryptologyCredentialIssuerWithFile("", 0)
	merkleBuilder := crypto.NewGnarkMerkleRootBuilder()
	logger := zap.NewNop()

	votingUC := usecase.NewVotingUseCase(
		voteRepo, participationRepo, credentialRepo, proposalRepo, sessionRepo,
		proofVerifier, encrypter, issuer, logger,
	)
	registerUC := usecase.NewRegisterUseCaseWithPool(
		credentialRepo, proposalRepo, issuer, claimRepo, logger,
	)
	proposalUC := usecase.NewProposalUseCase(proposalRepo, logger)
	participationUC := usecase.NewParticipationUseCase(participationRepo, credentialRepo, logger)
	tallyUC := usecase.NewTallyUseCase(
		voteRepo, proposalRepo, sessionRepo, resultRepo, anchorRepo,
		encrypter, crypto.NewBallotNullifierGenerator(), merkleBuilder, logger,
	)

	handler := httpapi.NewHandlerWithTallyAndMode(
		votingUC, registerUC, proposalUC, participationUC, tallyUC, true, logger,
	)
	server := httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          "test",
		AdminBearerToken: adminToken,
	}, handler, logger)

	recorded := &[]recordedRequest{}
	engine := server.Engine()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, hook := range interceptors {
			if hook(w, r) {
				return
			}
		}
		raw, _ := io.ReadAll(r.Body)
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(raw))
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		*recorded = append(*recorded, recordedRequest{Path: r.URL.Path, Body: body})
		engine.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	return ts, recorded
}

// createProposal 建一个提案，返回它的 id。
func createProposal(t *testing.T, ts *httptest.Server, adminToken string) string {
	t.Helper()
	resp := postAdminJSON(t, ts.URL+"/api/v1/proposals", adminToken, map[string]any{
		"title":     "Blind Signature Election",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	})
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposal struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposal))
	_ = resp.Body.Close()
	return proposal.ID
}

// TestAPI_TwoPhaseBlindSignature_E2E 验证真两段式盲签名的端到端流程。
//
// 这是路线图 T17 的验收 ①：客户端拿到的凭证，服务端在**签名阶段**只见盲化消息。
func TestAPI_TwoPhaseBlindSignature_E2E(t *testing.T) {
	const adminToken = "secret"
	ts, recorded := newBlindSignatureServer(t, adminToken)
	proposalID := createProposal(t, ts, adminToken)

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	userPubKey := []byte("one-time-public-key")
	credential, err := client.ClaimCredential(proposalID, userPubKey)
	require.NoError(t, err)
	require.NotEmpty(t, credential.UserID)
	require.NotEmpty(t, credential.Signature)
	require.Equal(t, base64.StdEncoding.EncodeToString(userPubKey), credential.UserPubKey)

	// ① 预留阶段：请求体里**没有公钥** —— 服务端一旦知道公钥，盲化就失效了。
	var reserveBody map[string]any
	var blindSignBody map[string]any
	for _, item := range *recorded {
		switch item.Path {
		case "/api/v1/proposals/" + proposalID + "/credentials/claim":
			reserveBody = item.Body
		case "/api/v1/proposals/" + proposalID + "/credentials/blind-sign":
			blindSignBody = item.Body
		}
	}
	require.NotNil(t, reserveBody, "必须走到预留这一步")
	require.NotContains(t, reserveBody, "user_pub_key", "预留阶段绝不能把公钥交给服务端")
	require.NotNil(t, blindSignBody, "必须走到盲签名这一步")

	// ② 盲签名阶段：服务端收到的是盲化消息，不是被签内容。
	blinded, err := base64.StdEncoding.DecodeString(blindSignBody["blinded_message"].(string))
	require.NoError(t, err)
	require.NotEmpty(t, blinded)
	message := crypto.CredentialMessage(credential.UserID, userPubKey)
	require.NotEqual(t, message, blinded, "服务端收到的必须是盲化后的消息")
	require.NotContains(t, string(blinded), string(message))
}

// TestAPI_BlindSignatureCrossAttack_E2E 验证交叉攻击失败。
//
// 这是路线图 T17 的验收 ②：拿 A 的盲签名去签 B，服务端验签必然失败。
func TestAPI_BlindSignatureCrossAttack_E2E(t *testing.T) {
	const adminToken = "secret"
	ts, _ := newBlindSignatureServer(t, adminToken)
	proposalID := createProposal(t, ts, adminToken)

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	// A 领一个槽位，拿到自己的盲签名。
	userPubKeyA := []byte("public-key-a")
	reservationA, err := client.Reserve(proposalID)
	require.NoError(t, err)
	blindedA, err := client.Blind(reservationA, userPubKeyA)
	require.NoError(t, err)
	blindSignatureA, err := client.BlindSignPool(
		proposalID, reservationA, blindedA.BlindedMessage,
	)
	require.NoError(t, err)
	signatureA, err := blindedA.Finalize(blindSignatureA)
	require.NoError(t, err)

	// 诚实用法成立：A 用自己的签名能过。
	credential, err := client.Attest(
		proposalID, reservationA.UserID, userPubKeyA, signatureA,
	)
	require.NoError(t, err)
	require.Equal(t, reservationA.UserID, credential.UserID)

	// B 领另一个槽位 —— A 的槽位已经被领走，所以 B 拿到的一定是另一个。
	// （预留不占位，所以必须等 A 真正 attest 完，B 才会拿到不同的槽位。）
	reservationB, err := client.Reserve(proposalID)
	require.NoError(t, err)
	require.NotEqual(t, reservationA.UserID, reservationB.UserID)

	// 交叉攻击 ①：拿 A 的签名去签 B 的身份 —— 验签必然失败。
	_, err = client.Attest(
		proposalID, reservationB.UserID, []byte("public-key-b"), signatureA,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "INVALID_CREDENTIAL")

	// 交叉攻击 ②：拿 A 的签名配 B 的公钥 —— 同样失败。
	_, err = client.Attest(
		proposalID, reservationB.UserID, []byte("public-key-b"), signatureA,
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "INVALID_CREDENTIAL")
}

// TestAPI_ClaimRetriesWhenSlotTaken_E2E 验证槽位被别人抢走时客户端会换一个槽位
// 重来。
//
// 服务端取槽位时**不占位**（占位会让「只预留不 attest」变成耗尽池的 DoS），所以
// 并发领取会撞车。撞车时必须由客户端重试 —— 重试要重新盲化（新的 user_id 意味
// 着新的消息），服务端代劳不了。
func TestAPI_ClaimRetriesWhenSlotTaken_E2E(t *testing.T) {
	const adminToken = "secret"

	var attestAttempts int
	var injectedConflicts int
	ts, recorded := newBlindSignatureServer(
		t,
		adminToken,
		func(w http.ResponseWriter, r *http.Request) bool {
			if !strings.HasSuffix(r.URL.Path, "/credentials/attest") {
				return false
			}
			attestAttempts++
			if attestAttempts > 1 {
				return false
			}
			// 第一次 attest 假装槽位被别人抢走。
			injectedConflicts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"code":"CREDENTIAL_EXISTS","error":"凭证已存在"}`))
			return true
		},
	)
	proposalID := createProposal(t, ts, adminToken)

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 2))

	credential, err := client.ClaimCredential(proposalID, []byte("one-time-public-key"))
	require.NoError(t, err)
	require.NotEmpty(t, credential.Signature)

	// 撞车了一次，重试后拿到凭证。
	require.Equal(t, 1, injectedConflicts)
	require.Equal(t, 2, attestAttempts, "第一次被挡下，第二次才落库")
	reserveCalls := 0
	for _, item := range *recorded {
		if strings.HasSuffix(item.Path, "/credentials/claim") {
			reserveCalls++
		}
	}
	require.Equal(t, 2, reserveCalls, "换一个槽位重来：预留应该被调用两次")
}

// TestAPI_BlindSignRequiresPoolSlot_E2E 验证公开的盲签名端点不能为「池里没有的
// user_id」签名 —— 否则签发方就是任意消息的签名预言机。
func TestAPI_BlindSignRequiresPoolSlot_E2E(t *testing.T) {
	const adminToken = "secret"
	ts, _ := newBlindSignatureServer(t, adminToken)
	proposalID := createProposal(t, ts, adminToken)

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = adminToken
	require.NoError(t, client.FillCredentialPool(proposalID, 1))

	resp := postJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/credentials/blind-sign",
		map[string]any{
			"user_id":            "not-in-pool",
			"blinded_message":    base64.StdEncoding.EncodeToString([]byte("blinded")),
			"issuer_key_version": 1,
		})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	_ = resp.Body.Close()
}
