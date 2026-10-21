package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/stretchr/testify/require"
)

// TestRun_BallotEncrypt_LocalEncryption 覆盖 CLI 的客户端本地加密命令。
//
// 关键性质：**服务端只提供公开的计票公钥，密文在客户端生成**。用例断言
// 落盘的密文能被对应的门限私钥解回客户端选的选项下标。
func TestRun_BallotEncrypt_LocalEncryption(t *testing.T) {
	t.Parallel()

	keys, err := ballot.GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := ballot.UncompressPoint(keys.Public)
	require.NoError(t, err)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/proposals":
			_, _ = w.Write([]byte(`{"items":[{"id":"p1","options":["A","B"]}]}`))
		case "/api/v1/proposals/p1/election-key":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"proposal_id": "p1",
				"public_key":  base64.StdEncoding.EncodeToString(uncompressed),
				"encoding":    "uncompressed",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	outPath := filepath.Join(t.TempDir(), "ballot.json")
	code := 0
	out, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL,
			"ballot-encrypt", "-proposal", "p1", "-option", "B", "-out", outPath,
		})
	})
	require.Equal(t, 0, code, "命令失败: %s", errOut)
	require.Contains(t, out, outPath)

	var encrypted ballotFile
	raw, err := os.ReadFile(outPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &encrypted))
	require.Equal(t, "p1", encrypted.ProposalID)
	require.Equal(t, "B", encrypted.SelectedOption)

	// 密文是合法的 v3 选票密文，且能解回选项 B（下标 1）。
	ciphertext, err := base64.StdEncoding.DecodeString(encrypted.EncryptedVote)
	require.NoError(t, err)
	require.NoError(t, ballot.ValidateBallotCiphertext(ciphertext))
	index, err := ballot.DecryptBallotOption(keys.Private, ciphertext, 2)
	require.NoError(t, err)
	require.Equal(t, uint64(1), index)

	// nullifier 是规范编码（64 位小写 hex），秘密只留在本地选票文件里。
	require.Len(t, encrypted.Nullifier, 64)
	require.NotEmpty(t, encrypted.NullifierSecret)
}
