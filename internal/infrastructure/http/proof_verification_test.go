package httpapi

import (
	"bytes"
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

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
	"github.com/stretchr/testify/require"
)

// proofMaterial 是一次真实 Groth16 证明所需的全部材料。
type proofMaterial struct {
	proof         []byte
	publicWitness []byte
	verifyingKey  []byte
}

// buildProofMaterial 跑一次真实的电路编译 + Setup + Prove，
// 产出 proof、public witness 与 verifying key。
func buildProofMaterial(t *testing.T, x int, y int) proofMaterial {
	t.Helper()

	var circuit serverTestCircuit
	ccs, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &circuit)
	require.NoError(t, err)
	pk, vk, err := groth16.Setup(ccs)
	require.NoError(t, err)

	assignment := serverTestCircuit{X: x, Y: y}
	fullWitness, err := frontend.NewWitness(&assignment, ecc.BN254)
	require.NoError(t, err)

	proof, err := groth16.Prove(ccs, pk, fullWitness)
	require.NoError(t, err)

	var proofBuf bytes.Buffer
	_, err = proof.WriteTo(&proofBuf)
	require.NoError(t, err)

	// public_witness 只放公开输入部分：gnark 的 groth16.Verify 要求 witness
	// 的大小正好等于公开输入个数，放整个 witness 会被拒。
	publicWitness, err := fullWitness.Public()
	require.NoError(t, err)
	witnessBytes, err := publicWitness.MarshalBinary()
	require.NoError(t, err)

	var vkBuf bytes.Buffer
	_, err = vk.WriteTo(&vkBuf)
	require.NoError(t, err)

	return proofMaterial{
		proof:         proofBuf.Bytes(),
		publicWitness: witnessBytes,
		verifyingKey:  vkBuf.Bytes(),
	}
}

// readErrorBody 读出错误响应的正文，用于断言失败时给出可读的原因。
func readErrorBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(raw)
}

// TestServer_RealProofVerification 跑通真实 gnark 验证的 HTTP 链路。
//
// 之前所有 HTTP 层测试用的都是空操作验证器（NewGnarkProofVerifier("")），
// 于是 public_witness 字段、PublicInputs()、错误到 HTTP 状态码的映射
// 全都没有被真正验证过：改坏 handler 丢掉 public_witness，CI 依然是绿的。
func TestServer_RealProofVerification(t *testing.T) {
	// 3^3 + 3 + 5 = 35，满足电路约束 Y = X^3 + X + 5。
	material := buildProofMaterial(t, 3, 35)

	vkPath := filepath.Join(t.TempDir(), "verifying.key")
	require.NoError(t, os.WriteFile(vkPath, material.verifyingKey, 0o600))

	verifier, err := cryptoinfra.NewGnarkProofVerifier(vkPath)
	require.NoError(t, err)

	engine, _, db := buildFullStackEngineWithVerifier(t, verifier)
	ts := httptest.NewServer(engine)
	defer ts.Close()

	proposalReq := map[string]any{
		"title":     "Board Election",
		"options":   []string{"candidate-A", "candidate-B"},
		"starts_at": time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339),
		"ends_at":   time.Now().UTC().Add(90 * time.Minute).Format(time.RFC3339),
	}
	resp := doPostAdminJSON(t, ts.URL+"/api/v1/proposals", "secret", proposalReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var proposal struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&proposal))
	_ = resp.Body.Close()

	credentialReq := map[string]any{
		"proposal_id":  proposal.ID,
		"user_id":      "u-1",
		"user_pub_key": base64.StdEncoding.EncodeToString([]byte("user-pub-key")),
	}
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/credentials", "secret", credentialReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var credential struct {
		UserPubKey string `json:"user_pub_key"`
		Signature  string `json:"signature"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&credential))
	_ = resp.Body.Close()

	// 解密会话必须在投票窗口开启之前初始化。
	setupReq := map[string]any{"threshold": 2, "trustees": 3}
	resp = doPostAdminJSON(t, ts.URL+"/api/v1/proposals/"+proposal.ID+"/decryption/setup", "secret", setupReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	// 把投票窗口调整到现在。
	require.NoError(t, db.Model(&entity.Proposal{}).
		Where("id = ?", proposal.ID).
		Updates(map[string]any{
			"starts_at": time.Now().UTC().Add(-5 * time.Minute),
			"ends_at":   time.Now().UTC().Add(30 * time.Minute),
		}).Error)

	draftReq := map[string]any{"option": "candidate-A"}
	resp = doPostJSON(t, ts.URL+"/api/v1/proposals/"+proposal.ID+"/ballot-draft", draftReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	var draft struct {
		EncryptedVote string `json:"encrypted_vote"`
		Nullifier     string `json:"nullifier"`
	}
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&draft))
	_ = resp.Body.Close()

	voteBody := func(extra map[string]any) map[string]any {
		body := map[string]any{
			"proposal_id":          proposal.ID,
			"encrypted_vote":       draft.EncryptedVote,
			"proof":                base64.StdEncoding.EncodeToString(material.proof),
			"nullifier":            draft.Nullifier,
			"user_id":              "u-1",
			"user_pub_key":         credential.UserPubKey,
			"credential_signature": credential.Signature,
		}
		for key, value := range extra {
			body[key] = value
		}
		return body
	}

	t.Run("有效 proof 与 public_witness 被接受", func(t *testing.T) {
		resp := doPostJSON(t, ts.URL+"/api/v1/votes", voteBody(map[string]any{
			"public_witness": base64.StdEncoding.EncodeToString(material.publicWitness),
		}))
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusCreated, resp.StatusCode, readErrorBody(t, resp))
	})

	t.Run("缺少 public_witness 返回 400", func(t *testing.T) {
		resp := doPostJSON(t, ts.URL+"/api/v1/votes", voteBody(map[string]any{
			"nullifier": "null-missing-witness",
		}))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		_ = resp.Body.Close()
	})

	t.Run("被篡改的 public_witness 返回 400", func(t *testing.T) {
		tampered := append([]byte(nil), material.publicWitness...)
		tampered[len(tampered)-1] ^= 0xff
		resp := doPostJSON(t, ts.URL+"/api/v1/votes", voteBody(map[string]any{
			"nullifier":      "null-tampered-witness",
			"public_witness": base64.StdEncoding.EncodeToString(tampered),
		}))
		require.Equal(t, http.StatusBadRequest, resp.StatusCode)
		_ = resp.Body.Close()
	})
}
