// Package tally 定义计票的**规范算法**：从「密文选票 + 计票私钥」出发，
// 得出计票结论、merkle 根与审计锚点哈希。
//
// 为什么要单独一个包：审计复算工具（cmd/audit-recompute）必须与服务端
// **逐字节同源** —— 两边各写一份算法，一旦漂移，复算结论就失去意义。
// 所以算法只有这一份：服务端的 usecase/tally.go 与审计工具都调用 Compute，
// 任何一方改了算法，另一方立刻跟着变。
//
// ⚠️ 这里的所有算法细节都是**冻结**的（见 docs/api-contract-v1.md）：
//   - merkle 叶子 = nullifier + "|" + cipher
//   - 规范计票 JSON = 候选名升序的 [{"candidate":...,"count":...}]
//   - 锚点哈希 = SHA-256(proposalID | base64(merkleRoot) | 规范计票 JSON)
//
// 改了任何一处，库里已发布的锚点就与重算结果对不上 —— 那是破坏性变更。
package tally

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Decryptor 把选票密文还原成选项下标。
type Decryptor interface {
	Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error)
}

// MerkleRootBuilder 由叶子列表构建 merkle 根。
type MerkleRootBuilder interface {
	BuildRoot(leaves [][]byte) ([]byte, error)
}

// Ballot 是一条待计票的选票。
type Ballot struct {
	// ID 是选票记录的标识，只用于报错定位，不参与任何计算。
	ID string
	// Nullifier 是选票的唯一标识（64 位小写 hex），参与 merkle 叶子计算。
	Nullifier string
	// Cipher 是 v3 选票密文。
	Cipher []byte
}

// Input 是计票所需的全部输入。
type Input struct {
	// ProposalID 是提案标识，参与锚点哈希计算。
	ProposalID string
	// Options 是提案的候选项，按下标对应密文里加密的选项。
	Options []string
	// Ballots 是本次计票的全部选票。
	Ballots []Ballot
	// PrivateKey 是门限还原出来的计票私钥。
	PrivateKey []byte
}

// Result 是计票结论。
type Result struct {
	// TotalVotes 是参与计票的选票数。
	TotalVotes int
	// Tally 是候选名到票数的映射。
	Tally map[string]int
	// TallyJSON 是规范计票 JSON（候选名升序）。
	TallyJSON string
	// MerkleRoot 是选票叶子构成的 merkle 根。
	MerkleRoot []byte
	// AnchorHash 是审计锚点哈希。
	AnchorHash string
}

// InvalidOptionError 表示密文里的选项下标不在提案的候选项范围内。
//
// 服务端要把它映射成 ErrInvalidRequest（HTTP 400），所以这里用一个带字段的
// 类型而不是裸 error：调用方能拿到原始下标与选票标识，自己拼出符合
// 既有口径的错误文案。
type InvalidOptionError struct {
	Index    int
	BallotID string
}

func (e *InvalidOptionError) Error() string {
	return fmt.Sprintf("option index %d in vote %s is not defined for proposal", e.Index, e.BallotID)
}

// DecryptError 表示某张票解不开。
//
// 用合法份额还原出的私钥解不开一张票 —— 这只可能是票本身坏了（密文被改过、
// 被截断）。所以它是**审计结论的一部分**，不是「工具跑不起来」。
type DecryptError struct {
	BallotID string
	Err      error
}

func (e *DecryptError) Error() string {
	return fmt.Sprintf("decrypt vote %s: %v", e.BallotID, e.Err)
}

func (e *DecryptError) Unwrap() error {
	return e.Err
}

// Compute 按规范算法重算计票结论。
//
// 与服务端的计票链路共用这一个函数 —— 这是「离线复算有意义」的前提。
func Compute(in Input, decryptor Decryptor, merkle MerkleRootBuilder) (*Result, error) {
	if decryptor == nil {
		return nil, errors.New("decryptor is required")
	}
	if merkle == nil {
		return nil, errors.New("merkle root builder is required")
	}
	if len(in.Options) == 0 {
		return nil, errors.New("proposal has no options")
	}

	tally := make(map[string]int)
	leaves := make([][]byte, 0, len(in.Ballots))
	for _, ballot := range in.Ballots {
		index, err := decryptor.Decrypt(in.PrivateKey, ballot.Cipher, len(in.Options))
		if err != nil {
			return nil, &DecryptError{BallotID: ballot.ID, Err: err}
		}
		if index < 0 || index >= len(in.Options) {
			return nil, &InvalidOptionError{Index: index, BallotID: ballot.ID}
		}
		tally[in.Options[index]]++
		leaves = append(leaves, VoteLeaf(ballot.Nullifier, ballot.Cipher))
	}
	if len(leaves) == 0 {
		leaves = append(leaves, []byte("empty-vote-set"))
	}

	merkleRoot, err := merkle.BuildRoot(leaves)
	if err != nil {
		return nil, fmt.Errorf("build merkle root: %w", err)
	}
	tallyJSON, err := MarshalCanonicalTally(tally)
	if err != nil {
		return nil, fmt.Errorf("marshal tally: %w", err)
	}

	return &Result{
		TotalVotes: len(in.Ballots),
		Tally:      tally,
		TallyJSON:  tallyJSON,
		MerkleRoot: merkleRoot,
		AnchorHash: AnchorHash(in.ProposalID, merkleRoot, tallyJSON),
	}, nil
}

// VoteLeaf 构造 merkle 叶子：nullifier + "|" + cipher。
func VoteLeaf(nullifier string, cipher []byte) []byte {
	leaf := make([]byte, 0, len(nullifier)+1+len(cipher))
	leaf = append(leaf, []byte(nullifier)...)
	leaf = append(leaf, '|')
	leaf = append(leaf, cipher...)
	return leaf
}

// AnchorHash 计算审计锚点哈希：SHA-256(proposalID | base64(merkleRoot) | 规范计票 JSON)。
func AnchorHash(proposalID string, merkleRoot []byte, tallyJSON string) string {
	hashInput := strings.Join([]string{
		proposalID,
		base64.StdEncoding.EncodeToString(merkleRoot),
		tallyJSON,
	}, "|")
	sum := sha256.Sum256([]byte(hashInput))
	return hex.EncodeToString(sum[:])
}

// Entry 是规范计票 JSON 的一项。
type Entry struct {
	Candidate string `json:"candidate"`
	Count     int    `json:"count"`
}

// MarshalCanonicalTally 把计票映射序列化成规范 JSON：候选名升序。
func MarshalCanonicalTally(tally map[string]int) (string, error) {
	keys := make([]string, 0, len(tally))
	for key := range tally {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]Entry, 0, len(keys))
	for _, key := range keys {
		items = append(items, Entry{
			Candidate: key,
			Count:     tally[key],
		})
	}

	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// UnmarshalCanonicalTally 解析规范计票 JSON。
func UnmarshalCanonicalTally(raw string) (map[string]int, error) {
	if raw == "" {
		return map[string]int{}, nil
	}
	var items []Entry
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, err
	}
	out := make(map[string]int, len(items))
	for _, item := range items {
		out[item.Candidate] = item.Count
	}
	return out, nil
}

// Published 是库里已发布的计票结论。
type Published struct {
	TotalVotes int
	TallyJSON  string
	MerkleRoot []byte
	AnchorHash string
}

// Mismatch 是一处不一致。
type Mismatch struct {
	Field      string `json:"field"`
	Published  string `json:"published"`
	Recomputed string `json:"recomputed"`
}

// Compare 逐字节比对复算结论与已发布结论。
//
// 返回空切片代表完全一致 —— 空切片与 nil 在这里等价，调用方用 len() 判断。
func Compare(published Published, recomputed Result) []Mismatch {
	mismatches := make([]Mismatch, 0, 4)
	appendMismatch := func(field, publishedValue, recomputedValue string) {
		if publishedValue != recomputedValue {
			mismatches = append(mismatches, Mismatch{
				Field:      field,
				Published:  publishedValue,
				Recomputed: recomputedValue,
			})
		}
	}

	appendMismatch("total_votes",
		strconv.Itoa(published.TotalVotes),
		strconv.Itoa(recomputed.TotalVotes),
	)
	appendMismatch("tally_json", published.TallyJSON, recomputed.TallyJSON)
	appendMismatch("merkle_root",
		hex.EncodeToString(published.MerkleRoot),
		hex.EncodeToString(recomputed.MerkleRoot),
	)
	appendMismatch("anchor_hash", published.AnchorHash, recomputed.AnchorHash)

	return mismatches
}
