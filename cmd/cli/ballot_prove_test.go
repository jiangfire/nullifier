package main

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/stretchr/testify/require"
)

// newElectionServer 起一个只提供「提案选项」与「计票公钥」的假服务端。
func newElectionServer(t *testing.T, proposalID string, options []string, uncompressedKey []byte) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/proposals", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"id": proposalID, "options": options}},
		})
	})
	mux.HandleFunc("/api/v1/proposals/"+proposalID+"/election-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"public_key": base64.StdEncoding.EncodeToString(uncompressedKey),
			"encoding":   "uncompressed",
		})
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestBallotProve_LocalProof 覆盖「本地加密 → 本地出证明 → 用服务端同一套
// 公开输入验过」这条链路。
//
// 它是 CLI 侧能力 A 的证据：proving key 只在客户端，出证明不需要服务端，
// 服务端拿到的公开输入与客户端逐字段相同（否则验签必然失败）。
func TestBallotProve_LocalProof(t *testing.T) {
	dir := t.TempDir()

	// 电路密钥：proving key 在客户端侧，verifying key 部署到服务端。
	provingKey, verifyingKey, err := ballot.GenerateKeys()
	require.NoError(t, err)
	provingKeyPath := filepath.Join(dir, "proving.key")
	verifyingKeyPath := filepath.Join(dir, "verifying.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))
	require.NoError(t, os.WriteFile(verifyingKeyPath, verifyingKey, 0o644))

	// 提案的计票密钥对（服务端持有私钥，客户端只有公钥）。
	electionKeys, err := ballot.GenerateKeyPair()
	require.NoError(t, err)
	uncompressedKey, err := ballot.UncompressPoint(electionKeys.Public)
	require.NoError(t, err)

	const proposalID = "p-1"
	options := []string{"candidate-A", "candidate-B"}
	srv := newElectionServer(t, proposalID, options, uncompressedKey)

	// 第一步：本地加密（选 candidate-B，下标 1）。
	ballotPath := filepath.Join(dir, "ballot.json")
	code := 0
	out, errOut := captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL,
			"ballot-encrypt", "-proposal", proposalID, "-option", "candidate-B", "-out", ballotPath,
		})
	})
	require.Equal(t, 0, code, "ballot-encrypt 失败: %s", errOut)
	require.Contains(t, out, ballotPath)

	var encrypted ballotFile
	raw, err := os.ReadFile(ballotPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &encrypted))
	require.Equal(t, proposalID, encrypted.ProposalID)
	require.NotEmpty(t, encrypted.EncryptedVote)
	require.NotEmpty(t, encrypted.Nullifier)
	require.NotEmpty(t, encrypted.NullifierSecret)
	require.NotEmpty(t, encrypted.Nonce)
	require.NotEmpty(t, encrypted.ElectionPublicKey)
	require.NotNil(t, encrypted.OptionIndex)
	require.Equal(t, 1, *encrypted.OptionIndex)
	require.Equal(t, 2, encrypted.NumOptions)

	// 第二步：本地出证明。
	provedPath := filepath.Join(dir, "proved.json")
	code = 0
	out, errOut = captureOutput(t, func() {
		code = run([]string{
			"-base-url", srv.URL,
			"ballot-prove", "-ballot-file", ballotPath,
			"-proving-key", provingKeyPath, "-out", provedPath,
		})
	})
	require.Equal(t, 0, code, "ballot-prove 失败: %s", errOut)
	require.Contains(t, out, provedPath)

	var proved ballotFile
	raw, err = os.ReadFile(provedPath)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &proved))
	require.NotEqual(t, encrypted.Proof, proved.Proof)

	proof, err := base64.StdEncoding.DecodeString(proved.Proof)
	require.NoError(t, err)
	require.NotEmpty(t, proof)

	// 第三步：用服务端同一套公开输入验签 —— 必须通过。
	inputs := publicInputsOf(t, proved)
	verifier, err := ballot.NewVerifier(verifyingKeyPath)
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(proof, inputs))

	// 换一个 nullifier，公开输入就变了 —— 同一份证明必须验不过。
	//
	// ⚠️ **改的是最低字节，不是最高字节**：域元素的规范编码要求值 < r，而 BN254
	// 的 r 顶字节是 0x30。nullifier 是 MiMC 的输出（均匀分布），顶字节恰好是
	// 0x30 的概率约 1% —— 这时翻掉最高字节的最低位就越界了，isCanonicalScalar
	// 会直接拒绝非规范编码，用例就会因为「篡改方式不合法」而红，而不是因为
	// 「验签没过」（2026-10-26 的 CI 上就是这样偶发变红的）。
	// 改最低字节则只差 ±1，不会越界。
	tampered := proved
	nullifierBytes, err := hex.DecodeString(proved.Nullifier)
	require.NoError(t, err)
	nullifierBytes[len(nullifierBytes)-1] ^= 0x01
	tampered.Nullifier = hex.EncodeToString(nullifierBytes)
	tamperedInputs := publicInputsOf(t, tampered)
	require.Error(t, verifier.Verify(proof, tamperedInputs))
}

// TestBallotProve_RejectsIncompleteBallot 覆盖缺材料时的报错。
func TestBallotProve_RejectsIncompleteBallot(t *testing.T) {
	dir := t.TempDir()
	provingKey, _, err := ballot.GenerateKeys()
	require.NoError(t, err)
	provingKeyPath := filepath.Join(dir, "proving.key")
	require.NoError(t, os.WriteFile(provingKeyPath, provingKey, 0o600))

	cases := []struct {
		name   string
		ballot ballotFile
	}{
		{
			name:   "缺 proposal_id",
			ballot: ballotFile{EncryptedVote: "x", OptionIndex: intPtr(0), NumOptions: 2},
		},
		{
			name:   "缺 option_index",
			ballot: ballotFile{ProposalID: "p-1", EncryptedVote: "x", NumOptions: 2},
		},
		{
			name:   "缺 num_options",
			ballot: ballotFile{ProposalID: "p-1", EncryptedVote: "x", OptionIndex: intPtr(0)},
		},
		{
			name: "缺 election_public_key",
			ballot: ballotFile{
				ProposalID: "p-1", EncryptedVote: "x",
				OptionIndex: intPtr(0), NumOptions: 2,
			},
		},
		{
			name: "缺 nonce",
			ballot: ballotFile{
				ProposalID: "p-1", EncryptedVote: "x", OptionIndex: intPtr(0),
				NumOptions: 2, ElectionPublicKey: "k",
			},
		},
		{
			name: "缺 nullifier_secret",
			ballot: ballotFile{
				ProposalID: "p-1", EncryptedVote: "x", OptionIndex: intPtr(0),
				NumOptions: 2, ElectionPublicKey: "k", Nonce: "n",
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(dir, testCase.name+".json")
			require.NoError(t, writeJSON(path, testCase.ballot))

			code := 0
			_, errOut := captureOutput(t, func() {
				code = run([]string{
					"ballot-prove", "-ballot-file", path, "-proving-key", provingKeyPath,
				})
			})
			require.Equal(t, 1, code)
			require.NotEmpty(t, errOut)
		})
	}
}

// publicInputsOf 用与服务端相同的入口拼公开输入。
func publicInputsOf(t *testing.T, bf ballotFile) ballot.PublicInputs {
	t.Helper()

	nullifier, err := hex.DecodeString(bf.Nullifier)
	require.NoError(t, err)
	compressed, err := ballot.CompressPoint(mustDecode(t, bf.ElectionPublicKey))
	require.NoError(t, err)
	inputs, err := ballot.BuildPublicInputs(entity.BallotStatement{
		ProposalID:        bf.ProposalID,
		Nullifier:         nullifier,
		ElectionPublicKey: compressed,
		Ciphertext:        mustDecode(t, bf.EncryptedVote),
		NumOptions:        bf.NumOptions,
	})
	require.NoError(t, err)
	return inputs
}

func mustDecode(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(value)
	require.NoError(t, err)
	return raw
}

func intPtr(value int) *int {
	return &value
}
