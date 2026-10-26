package tally

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeDecryptor 把密文的第一个字节当成选项下标 —— 只为测试编排逻辑，
// 真实的解密在 cmd/audit-recompute 的端到端用例里走真密码学。
type fakeDecryptor struct {
	err error
}

func (f fakeDecryptor) Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error) {
	if f.err != nil {
		return 0, f.err
	}
	if len(ciphertext) == 0 {
		return 0, errors.New("empty ciphertext")
	}
	return int(ciphertext[0]) % numOptions, nil
}

// fakeMerkle 把叶子按顺序哈希后取第一个 32 字节 —— 够用来验证叶子顺序与
// 「叶子内容变了根就变了」这两件事。
type fakeMerkle struct {
	leaves [][]byte
}

func (f *fakeMerkle) BuildRoot(leaves [][]byte) ([]byte, error) {
	f.leaves = leaves
	if len(leaves) == 0 {
		return nil, errors.New("leaves are empty")
	}
	h := sha256.New()
	for _, leaf := range leaves {
		h.Write(leaf)
	}
	return h.Sum(nil), nil
}

func TestCompute_CountsAndOrdersTally(t *testing.T) {
	t.Parallel()

	merkle := &fakeMerkle{}
	result, err := Compute(Input{
		ProposalID: "p1",
		Options:    []string{"A", "B"},
		Ballots: []Ballot{
			{ID: "v1", Nullifier: "n1", Cipher: []byte{0}},
			{ID: "v2", Nullifier: "n2", Cipher: []byte{1}},
			{ID: "v3", Nullifier: "n3", Cipher: []byte{0}},
		},
		PrivateKey: []byte("key"),
	}, fakeDecryptor{}, merkle)
	require.NoError(t, err)

	assert.Equal(t, 3, result.TotalVotes)
	assert.Equal(t, map[string]int{"A": 2, "B": 1}, result.Tally)
	assert.Equal(t, `[{"candidate":"A","count":2},{"candidate":"B","count":1}]`, result.TallyJSON)
	require.Len(t, merkle.leaves, 3)
	assert.Equal(t, "n1|"+string([]byte{0}), string(merkle.leaves[0]))
}

func TestCompute_EmptyVoteSetUsesPlaceholderLeaf(t *testing.T) {
	t.Parallel()

	merkle := &fakeMerkle{}
	result, err := Compute(Input{
		ProposalID: "p1",
		Options:    []string{"A"},
	}, fakeDecryptor{}, merkle)
	require.NoError(t, err)

	assert.Equal(t, 0, result.TotalVotes)
	require.Len(t, merkle.leaves, 1)
	assert.Equal(t, "empty-vote-set", string(merkle.leaves[0]))
}

func TestCompute_RejectsInvalidOptionIndex(t *testing.T) {
	t.Parallel()

	_, err := Compute(Input{
		ProposalID: "p1",
		Options:    []string{"A"},
		Ballots:    []Ballot{{ID: "v1", Nullifier: "n1", Cipher: []byte{7}}},
	}, fakeDecryptor{err: nil}, &fakeMerkle{})
	require.NoError(t, err)

	// 越界的下标要能被识别出来：直接构造一个越界的解密结果。
	_, err = Compute(Input{
		ProposalID: "p1",
		Options:    []string{"A"},
		Ballots:    []Ballot{{ID: "v1", Nullifier: "n1", Cipher: []byte{0}}},
	}, outOfRangeDecryptor{}, &fakeMerkle{})
	require.Error(t, err)

	var invalidOption *InvalidOptionError
	require.True(t, errors.As(err, &invalidOption))
	assert.Equal(t, 5, invalidOption.Index)
	assert.Equal(t, "v1", invalidOption.BallotID)
	assert.Contains(t, err.Error(), "is not defined for proposal")
}

// outOfRangeDecryptor 永远返回一个越界的下标。
type outOfRangeDecryptor struct{}

func (outOfRangeDecryptor) Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error) {
	return 5, nil
}

func TestCompute_PropagatesDecryptError(t *testing.T) {
	t.Parallel()

	boom := errors.New("boom")
	_, err := Compute(Input{
		ProposalID: "p1",
		Options:    []string{"A"},
		Ballots:    []Ballot{{ID: "v1", Nullifier: "n1", Cipher: []byte{0}}},
	}, fakeDecryptor{err: boom}, &fakeMerkle{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "decrypt vote v1")

	// 解不开的票必须能被识别出来 —— 审计工具据此判「审计未通过」，
	// 而不是「工具跑不动」。
	var decryptErr *DecryptError
	require.True(t, errors.As(err, &decryptErr))
	assert.Equal(t, "v1", decryptErr.BallotID)
	// 底层错误要能穿透上来（服务端靠它判断错误来源）。
	require.ErrorIs(t, err, boom)
}

func TestCompute_RequiresDependencies(t *testing.T) {
	t.Parallel()

	_, err := Compute(Input{Options: []string{"A"}}, nil, &fakeMerkle{})
	require.Error(t, err)

	_, err = Compute(Input{Options: []string{"A"}}, fakeDecryptor{}, nil)
	require.Error(t, err)

	_, err = Compute(Input{}, fakeDecryptor{}, &fakeMerkle{})
	require.Error(t, err)
}

// TestAnchorHash_KAT 钉死锚点哈希的算法口径。
//
// 改了这条公式，库里已发布的锚点就与重算结果对不上 —— 那是破坏性变更，
// 必须显式改这里并走下一个 minor 版本。
func TestAnchorHash_KAT(t *testing.T) {
	t.Parallel()

	merkleRoot := []byte{0x01, 0x02, 0x03}
	tallyJSON := `[{"candidate":"A","count":2}]`

	got := AnchorHash("p1", merkleRoot, tallyJSON)
	assert.Equal(t, "0987f3c95a37b1c73168e266c146c2a2a7300593c24a415dde9f3d90f7920707", got)

	// 输入的任何一项变化都必须改掉哈希。
	assert.NotEqual(t, got, AnchorHash("p2", merkleRoot, tallyJSON))
	assert.NotEqual(t, got, AnchorHash("p1", []byte{0x01, 0x02, 0x04}, tallyJSON))
	assert.NotEqual(t, got, AnchorHash("p1", merkleRoot, `[{"candidate":"A","count":3}]`))
}

func TestVoteLeaf(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "abc|xyz", string(VoteLeaf("abc", []byte("xyz"))))
}

func TestMarshalCanonicalTally_SortsCandidates(t *testing.T) {
	t.Parallel()

	raw, err := MarshalCanonicalTally(map[string]int{"B": 1, "A": 2})
	require.NoError(t, err)
	assert.Equal(t, `[{"candidate":"A","count":2},{"candidate":"B","count":1}]`, raw)

	empty, err := MarshalCanonicalTally(map[string]int{})
	require.NoError(t, err)
	assert.Equal(t, `[]`, empty)
}

func TestUnmarshalCanonicalTally(t *testing.T) {
	t.Parallel()

	parsed, err := UnmarshalCanonicalTally(`[{"candidate":"A","count":1}]`)
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"A": 1}, parsed)

	empty, err := UnmarshalCanonicalTally("")
	require.NoError(t, err)
	assert.Empty(t, empty)

	_, err = UnmarshalCanonicalTally(`{bad-json`)
	require.Error(t, err)
}

func TestCompare_ReportsEveryMismatch(t *testing.T) {
	t.Parallel()

	published := Published{
		TotalVotes: 2,
		TallyJSON:  `[{"candidate":"A","count":2}]`,
		MerkleRoot: []byte{0x01},
		AnchorHash: "anchor",
	}
	recomputed := Result{
		TotalVotes: 2,
		TallyJSON:  `[{"candidate":"A","count":2}]`,
		MerkleRoot: []byte{0x01},
		AnchorHash: "anchor",
	}

	assert.Empty(t, Compare(published, recomputed))

	recomputed.TallyJSON = `[{"candidate":"A","count":1}]`
	recomputed.TotalVotes = 3
	recomputed.MerkleRoot = []byte{0x02}
	recomputed.AnchorHash = "other"

	mismatches := Compare(published, recomputed)
	require.Len(t, mismatches, 4)

	fields := make([]string, 0, len(mismatches))
	for _, mismatch := range mismatches {
		fields = append(fields, mismatch.Field)
	}
	assert.ElementsMatch(t,
		[]string{"total_votes", "tally_json", "merkle_root", "anchor_hash"},
		fields,
	)
}

func TestCompare_MerkleRootIsHexEncoded(t *testing.T) {
	t.Parallel()

	published := Published{MerkleRoot: []byte{0xde, 0xad}}
	recomputed := Result{MerkleRoot: []byte{0xde, 0xad}}

	assert.Empty(t, Compare(published, recomputed))

	mismatches := Compare(published, Result{MerkleRoot: []byte{0xbe, 0xef}})
	require.Len(t, mismatches, 1)
	assert.Equal(t, "dead", mismatches[0].Published)
	assert.Equal(t, "beef", mismatches[0].Recomputed)
	assert.Equal(t, hex.EncodeToString([]byte{0xde, 0xad}), "dead")
}
