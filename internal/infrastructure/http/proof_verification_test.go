package httpapi

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	cryptoinfra "anonvote/internal/infrastructure/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/tests/testutil"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// readErrorBody 读出错误响应的正文，用于断言失败时给出可读的原因。
func readErrorBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(raw)
}

// ballotKeys 是电路密钥与其落盘路径。
type ballotKeys struct {
	verifyingKeyPath string
	provingKeyPath   string
}

// generateBallotKeys 生成选票电路的密钥并落盘。
//
// verifying key 给服务端验签，proving key 只留在客户端侧。
func generateBallotKeys(t *testing.T) ballotKeys {
	t.Helper()

	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)

	dir := t.TempDir()
	keys := ballotKeys{
		verifyingKeyPath: filepath.Join(dir, "verifying.key"),
		provingKeyPath:   filepath.Join(dir, "proving.key"),
	}
	require.NoError(t, os.WriteFile(keys.verifyingKeyPath, verifyingKey, 0o600))
	require.NoError(t, os.WriteFile(keys.provingKeyPath, provingKey, 0o600))
	return keys
}

// clientBallot 是客户端侧生成的一份选票：密文、nullifier 与证明。
type clientBallot struct {
	ciphertext []byte
	nullifier  string
	proof      []byte
}

// buildClientBallot 在客户端侧完成「选票 → 加密 → 证明」的全流程。
func buildClientBallot(
	t *testing.T,
	prover *ballot.Prover,
	proposalID string,
	electionPublicKey []byte,
	optionIndex uint64,
	numOptions int,
) clientBallot {
	t.Helper()

	secret, err := ballot.GenerateNullifierSecret()
	require.NoError(t, err)
	var secretElement fr.Element
	secretElement.SetBytes(secret)

	nullifier := ballot.DeriveNullifier(secretElement, ballot.DeriveProposalField(proposalID))
	nullifierBytes := nullifier.Bytes()
	ciphertext, nonce, err := ballot.EncryptBallot(electionPublicKey, optionIndex)
	require.NoError(t, err)

	statement := entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         nullifierBytes[:],
		ElectionPublicKey: electionPublicKey,
		Ciphertext:        ciphertext,
		NumOptions:        numOptions,
	}
	inputs, err := ballot.BuildPublicInputs(statement)
	require.NoError(t, err)

	proof, err := prover.Prove(inputs, optionIndex, nonce, secretElement)
	require.NoError(t, err)

	return clientBallot{
		ciphertext: ciphertext,
		nullifier:  ballot.EncodeScalarHex(nullifier),
		proof:      proof,
	}
}

// TestServer_RealBallotProof 跑通真实选票电路的 HTTP 链路。
//
// 这是 v0.2.0 能力 B 与能力 C 的验收证据：真实 Groth16 证明被服务端接受，
// 而任何一处公开输入被改动都必须被拒（400）。
func TestServer_RealBallotProof(t *testing.T) {
	keys := generateBallotKeys(t)
	verifier, err := cryptoinfra.NewGnarkProofVerifier(keys.verifyingKeyPath)
	require.NoError(t, err)
	prover, err := ballot.NewProver(keys.provingKeyPath)
	require.NoError(t, err)

	engine, _, db := buildFullStackEngineWithVerifier(t, verifier)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	// 建两个提案：跨提案复用证明要靠第二个提案验证。
	proposalID := createOpenProposal(t, ts, "Board Election")
	otherProposalID := createOpenProposal(t, ts, "Other Election")

	credential := issueCredential(t, ts, proposalID, "u-1")
	otherCredential := issueCredential(t, ts, otherProposalID, "u-2")

	publicKey := setupDecryptionSession(t, ts, proposalID)
	// 第二个提案也需要解密会话：没有会话时请求会先被「会话未就绪」拦下，
	// 那样就测不到「证明跨提案复用」这条路径了。
	setupDecryptionSession(t, ts, otherProposalID)

	openVotingWindow(t, db, proposalID)
	openVotingWindow(t, db, otherProposalID)

	ballot := buildClientBallot(t, prover, proposalID, publicKey, 0, 2)

	// 提案 ID 只从路径来（请求体里没有它）—— 见 docs/api-contract-v1.md。
	voteBody := func(cred credentialMaterial, b clientBallot) map[string]any {
		return map[string]any{
			"encrypted_vote":       base64.StdEncoding.EncodeToString(b.ciphertext),
			"proof":                base64.StdEncoding.EncodeToString(b.proof),
			"nullifier":            b.nullifier,
			"user_id":              cred.UserID,
			"user_pub_key":         cred.UserPubKey,
			"credential_signature": cred.Signature,
		}
	}

	t.Run("真实证明被接受", func(t *testing.T) {
		resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/votes", voteBody(credential, ballot))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode, readErrorBody(t, resp))
	})

	t.Run("跨提案复用证明返回 400", func(t *testing.T) {
		resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+otherProposalID+"/votes", voteBody(otherCredential, ballot))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("篡改 nullifier 返回 400", func(t *testing.T) {
		tampered := ballot
		tampered.nullifier = flipHexNibble(tampered.nullifier)

		resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/votes", voteBody(credential, tampered))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("篡改密文返回 400", func(t *testing.T) {
		tampered := ballot
		tampered.ciphertext = append([]byte(nil), ballot.ciphertext...)
		tampered.ciphertext[len(tampered.ciphertext)-1] ^= 0xff

		resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/votes", voteBody(credential, tampered))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("伪造的凭证签名返回 401", func(t *testing.T) {
		forged := credential
		forged.Signature = base64.StdEncoding.EncodeToString([]byte("forged-signature"))

		resp := doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposalID+"/votes", voteBody(forged, ballot))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// credentialMaterial 是一次投票要带的凭证材料。
type credentialMaterial struct {
	UserID     string
	UserPubKey string
	Signature  string
}

// flipHexNibble 翻转十六进制字符串最后一个字符的最低位，用来模拟篡改。
func flipHexNibble(value string) string {
	if value == "" {
		return value
	}
	last := value[len(value)-1]
	replacement := byte('0')
	if last == '0' {
		replacement = '1'
	}
	return value[:len(value)-1] + string(replacement)
}

// createOpenProposal 建一个处于投票窗口内的提案，返回提案 ID。
func createOpenProposal(t *testing.T, ts *httptest.Server, title string) string {
	t.Helper()

	req := map[string]any{
		"title":     title,
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	}
	resp := doPostAdminJSON(t, ts.URL+"/api/v1/proposals", "secret", req)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var proposal struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposal))
	return proposal.ID
}

// issueCredential 给用户签发一张凭证（走真两段式盲签名）。
func issueCredential(t *testing.T, ts *httptest.Server, proposalID, userID string) credentialMaterial {
	t.Helper()

	client := testutil.NewBlindClient(ts.URL)
	client.AdminToken = "secret"
	userPubKey := []byte("user-pub-key-" + userID)
	credential, err := client.IssueCredential(proposalID, userID, userPubKey)
	require.NoError(t, err)
	return credentialMaterial{
		UserID:     userID,
		UserPubKey: credential.UserPubKey,
		Signature:  credential.Signature,
	}
}

// setupDecryptionSession 初始化门限解密会话，返回提案的计票公钥。
func setupDecryptionSession(t *testing.T, ts *httptest.Server, proposalID string) []byte {
	t.Helper()

	resp := doPostAdminJSON(
		t,
		ts.URL+"/api/v1/proposals/"+proposalID+"/decryption/setup",
		"secret",
		map[string]any{"threshold": 2, "trustees": 3},
	)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusCreated, resp.StatusCode)

	var setup struct {
		PublicKey string `json:"public_key"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&setup))
	publicKey, err := base64.StdEncoding.DecodeString(setup.PublicKey)
	require.NoError(t, err)
	return publicKey
}

// openVotingWindow 把提案的投票窗口调整到现在。
func openVotingWindow(t *testing.T, db *gorm.DB, proposalID string) {
	t.Helper()

	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposalID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)
}
