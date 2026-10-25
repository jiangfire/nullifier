package ballot

import (
	"testing"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

// TestProveBallotProducesVerifiableProof 是客户端一站式出票的往返测试。
//
// 它验证的是浏览器端 prover 依赖的那条性质：客户端自己拼出来的公开输入，
// 与服务端按请求字段自拼出来的那一份逐字段相同 —— 否则证明验不过。
func TestProveBallotProducesVerifiableProof(t *testing.T) {
	pkPath, vkPath := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)
	verifier, err := NewVerifier(vkPath)
	require.NoError(t, err)

	election, err := GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := UncompressPoint(election.Public)
	require.NoError(t, err)

	const proposalID = "proposal-client-1"
	const numOptions = 3
	const optionIndex = uint64(2)

	result, err := prover.ProveBallot(ClientBallotRequest{
		ProposalID:        proposalID,
		ElectionPublicKey: uncompressed,
		OptionIndex:       optionIndex,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)
	require.Len(t, result.Ciphertext, CipherSize)
	require.Len(t, result.Nullifier, ScalarSize)
	require.NotEmpty(t, result.Proof)

	// 服务端侧的公开输入：按请求字段自拼。
	compressed, err := CompressPoint(uncompressed)
	require.NoError(t, err)
	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        proposalID,
		Nullifier:         result.Nullifier,
		ElectionPublicKey: compressed,
		Ciphertext:        result.Ciphertext,
		NumOptions:        numOptions,
	})
	require.NoError(t, err)
	require.NoError(t, verifier.Verify(result.Proof, inputs))

	// 计票侧能还原出客户端选的那个选项。
	decrypted, err := DecryptBallotOption(election.Private, result.Ciphertext, numOptions)
	require.NoError(t, err)
	require.Equal(t, optionIndex, decrypted)
}

// TestProveBallotRejectsInvalidInput 覆盖客户端请求里的各种非法取值。
func TestProveBallotRejectsInvalidInput(t *testing.T) {
	pkPath, _ := writeTestKeys(t)
	prover, err := NewProver(pkPath)
	require.NoError(t, err)

	election, err := GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := UncompressPoint(election.Public)
	require.NoError(t, err)

	cases := []struct {
		name    string
		request ClientBallotRequest
	}{
		{
			name: "选项下标越界",
			request: ClientBallotRequest{
				ProposalID:        "p",
				ElectionPublicKey: uncompressed,
				OptionIndex:       3,
				NumOptions:        3,
			},
		},
		{
			name: "选项数为零",
			request: ClientBallotRequest{
				ProposalID:        "p",
				ElectionPublicKey: uncompressed,
				OptionIndex:       0,
				NumOptions:        0,
			},
		},
		{
			name: "选项数超过上限",
			request: ClientBallotRequest{
				ProposalID:        "p",
				ElectionPublicKey: uncompressed,
				OptionIndex:       0,
				NumOptions:        MaxOptions + 1,
			},
		},
		{
			name: "计票公钥长度不对",
			request: ClientBallotRequest{
				ProposalID:        "p",
				ElectionPublicKey: []byte{1, 2, 3},
				OptionIndex:       0,
				NumOptions:        3,
			},
		},
		{
			name: "计票公钥不在曲线上",
			request: ClientBallotRequest{
				ProposalID:        "p",
				ElectionPublicKey: make([]byte, 2*PointSize),
				OptionIndex:       0,
				NumOptions:        3,
			},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := prover.ProveBallot(testCase.request)
			require.Error(t, err)
		})
	}
}

// TestNewProverFromBytes 覆盖从字节流加载证明器的入口：浏览器端没有文件路径。
func TestNewProverFromBytes(t *testing.T) {
	_, err := NewProverFromBytes(nil)
	require.Error(t, err)

	_, err = NewProverFromBytes([]byte("not a proving key"))
	require.Error(t, err)

	provingKey, _, err := GenerateKeys()
	require.NoError(t, err)
	prover, err := NewProverFromBytes(provingKey)
	require.NoError(t, err)
	require.NotNil(t, prover)
}
