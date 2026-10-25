package ballot

import (
	"fmt"

	"anonvote/internal/domain/entity"
)

// ClientBallotRequest 是客户端生成一张可提交选票所需的输入。
type ClientBallotRequest struct {
	// ProposalID 是提案 ID 原文。
	ProposalID string
	// ElectionPublicKey 是提案的计票公钥，**未压缩**形式（X 与 Y 各 32 字节大端）。
	ElectionPublicKey []byte
	// OptionIndex 是投票人选中的选项下标。
	OptionIndex uint64
	// NumOptions 是提案的选项数，也是电路的公开输入之一。
	NumOptions int
}

// ClientBallotResult 是一张客户端本地生成的选票。
type ClientBallotResult struct {
	// Ciphertext 是 v3 选票密文。
	Ciphertext []byte
	// Nullifier 是 32 字节的规范域元素。
	Nullifier []byte
	// Proof 是 Groth16 证明。
	Proof []byte
}

// ProveBallot 在客户端本地生成一张可提交的选票：加密、派生 nullifier、出证明。
//
// 三件事放在同一次调用里是有意的：nonce 与 nullifier 秘密这两样见证材料
// 只在本地出现一次，出了这个函数就再也拼不出与密文匹配的见证 —— 服务端
// （以及任何中间人）拿到的只有密文、nullifier 与证明本身。
//
// nullifier 秘密在函数内部随机生成且不返回：拿到它的人可以伪造出同一个
// nullifier 的第二张票，而这张票对投票人是不利的（服务端会判定重复投票），
// 因此没有把它交出去的必要。
func (p *Prover) ProveBallot(request ClientBallotRequest) (*ClientBallotResult, error) {
	if request.NumOptions <= 0 || request.NumOptions > MaxOptions {
		return nil, fmt.Errorf(
			"%w: invalid option count: %d", ErrInvalidCiphertext, request.NumOptions,
		)
	}
	if request.OptionIndex >= uint64(request.NumOptions) {
		return nil, fmt.Errorf(
			"%w: option index out of range: %d", ErrInvalidCiphertext, request.OptionIndex,
		)
	}
	// 公开输入要用压缩形式的计票公钥，客户端拿到的是未压缩形式。
	compressedKey, err := CompressPoint(request.ElectionPublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: election public key: %v", ErrInvalidCiphertext, err)
	}
	ciphertext, nonce, err := EncryptBallotWithUncompressedKey(
		request.ElectionPublicKey, request.OptionIndex,
	)
	if err != nil {
		return nil, err
	}
	secret, err := randomScalar()
	if err != nil {
		return nil, err
	}
	nullifier := DeriveNullifier(secret, DeriveProposalField(request.ProposalID))
	nullifierBytes := nullifier.Bytes()

	inputs, err := BuildPublicInputs(entity.BallotStatement{
		ProposalID:        request.ProposalID,
		Nullifier:         nullifierBytes[:],
		ElectionPublicKey: compressedKey,
		Ciphertext:        ciphertext,
		NumOptions:        request.NumOptions,
	})
	if err != nil {
		return nil, err
	}
	proof, err := p.Prove(inputs, request.OptionIndex, nonce, secret)
	if err != nil {
		return nil, err
	}
	return &ClientBallotResult{
		Ciphertext: ciphertext,
		Nullifier:  append([]byte(nil), nullifierBytes[:]...),
		Proof:      proof,
	}, nil
}
