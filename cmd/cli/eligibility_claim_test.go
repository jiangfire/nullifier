package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// TestRun_CredentialClaim_WithEligibility 覆盖「带资格证明领取凭证」的 CLI 流程。
//
// 关键性质：① 先取挑战、再出证明、最后带证明领取；② 请求体里的资格材料
// 只有三样，**没有真人身份字段**；③ proving key 只在本地，服务端只拿到
// 证明本身。
func TestRun_CredentialClaim_WithEligibility(t *testing.T) {
	t.Parallel()

	server := newBlindSigningServer(t)
	root, pack := buildTestMemberPackage(t, 1)
	provingKeyPath, verifier := testEligibilityKeys(t)
	packPath := filepath.Join(t.TempDir(), "eligibility.json")
	require.NoError(t, os.WriteFile(packPath, mustJSON(t, pack), 0o600))

	var claimBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		server.requests = append(server.requests, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/proposals/p1/credentials/eligibility-challenge":
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"proposal_id": "p1",
				"root":        root,
				"depth":       eligibility.TreeDepth,
				"challenge":   testChallengeHex,
			})
		case "/api/v1/proposals/p1/credentials/claim":
			_ = json.NewDecoder(r.Body).Decode(&claimBody)
			server.writeReservation(t, w, "p1", "slot-abc")
		case "/api/v1/proposals/p1/credentials/blind-sign":
			server.writeBlindSignature(t, w, r)
		default:
			server.writeCredential(t, w, r)
		}
	}))
	t.Cleanup(srv.Close)

	outPath := filepath.Join(t.TempDir(), "cred.json")
	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL,
			"credential-claim", "-proposal", "p1",
			"-eligibility", packPath,
			"-proving-key", provingKeyPath,
			"-out", outPath,
		})
	})
	require.Equal(t, 0, code, "命令失败: %s", errOut)

	// 三步：挑战 → 领取 → 盲签名 → attest。
	require.Equal(t, []string{
		"/api/v1/proposals/p1/credentials/eligibility-challenge",
		"/api/v1/proposals/p1/credentials/claim",
		"/api/v1/proposals/p1/credentials/blind-sign",
		"/api/v1/proposals/p1/credentials/attest",
	}, server.requests)

	// 领取请求体里只有资格材料那三样。
	require.NotNil(t, claimBody)
	require.Len(t, claimBody, 3)
	require.NotEmpty(t, claimBody["eligibility_proof"])
	require.NotEmpty(t, claimBody["qualification_id"])
	require.Equal(t, testChallengeHex, claimBody["challenge"])

	// 证明是真证明：服务端那边用同一套验证密钥能验过。
	proof, err := base64.StdEncoding.DecodeString(claimBody["eligibility_proof"].(string))
	require.NoError(t, err)
	statement, err := eligibility.BuildStatement(
		root,
		"p1",
		testChallengeHex,
		claimBody["qualification_id"].(string),
	)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(proof, statement))
}

// TestRun_CredentialClaim_EligibilityRequiresProvingKey 验证带资格包却没给
// proving key 时直接报错 —— 不能悄悄降级成「不带资格领取」。
func TestRun_CredentialClaim_EligibilityRequiresProvingKey(t *testing.T) {
	t.Parallel()

	_, pack := buildTestMemberPackage(t, 1)
	packPath := filepath.Join(t.TempDir(), "eligibility.json")
	require.NoError(t, os.WriteFile(packPath, mustJSON(t, pack), 0o600))

	code := 0
	_, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", "http://127.0.0.1:1",
			"credential-claim", "-proposal", "p1", "-eligibility", packPath,
		})
	})
	require.Equal(t, 1, code)
	require.Contains(t, errOut, "-proving-key")
}

// testChallengeHex 是测试用的领取挑战（十六进制的域元素）。
const testChallengeHex = "00aabbccddeeff00112233445566778899aabbccddeeff001122334455667788"

// buildTestMemberPackage 造一份真实的成员资格包，返回名单根与成员包。
func buildTestMemberPackage(t *testing.T, count int) (string, eligibility.MemberPackage) {
	t.Helper()

	raw := make([]byte, eligibility.FieldSize)
	_, err := rand.Read(raw)
	require.NoError(t, err)
	secret, err := eligibility.CanonicalSecret(raw)
	require.NoError(t, err)

	leaf, err := eligibility.DeriveLeaf(secret)
	require.NoError(t, err)
	leaves := []fr.Element{leaf}
	rootElement, paths, err := eligibility.BuildTree(leaves, eligibility.TreeDepth)
	require.NoError(t, err)
	root := eligibility.EncodeField(rootElement)

	path := make([]string, len(paths[0]))
	for i, element := range paths[0] {
		path[i] = eligibility.EncodeField(element)
	}

	return root, eligibility.MemberPackage{
		MemberID: "member-0",
		Secret:   hex.EncodeToString(secret),
		Leaf:     eligibility.EncodeField(leaf),
		Index:    0,
		Path:     path,
		Root:     root,
		Depth:    eligibility.TreeDepth,
	}
}

// testEligibilityKeys 生成一对资格密钥：proving key 落盘（客户端用），
// verifying key 留在测试里（服务端用）。
//
// 必须是**同一对** —— 两套 setup 出来的密钥互不相认。
func testEligibilityKeys(t *testing.T) (string, *eligibility.Verifier) {
	t.Helper()
	provingKey, verifyingKey, err := eligibility.GenerateKeys()
	require.NoError(t, err)
	provingKeyPath := filepath.Join(t.TempDir(), "eligibility-proving.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	verifier, err := eligibility.NewVerifierFromBytes(verifyingKey)
	require.NoError(t, err)
	return provingKeyPath, verifier
}

// mustJSON 把对象编码成 JSON 字节。
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return raw
}
