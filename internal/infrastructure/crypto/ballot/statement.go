package ballot

import (
	"fmt"

	"anonvote/internal/domain/entity"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// PublicInputs 是选票电路公开输入的域元素形式。
type PublicInputs struct {
	// ProposalID 是提案 ID 映射出的域元素。
	ProposalID fr.Element
	// Nullifier 是 nullifier 域元素。
	Nullifier fr.Element
	// ElectionKey 是提案的计票公钥。
	ElectionKey Point
	// CipherA 是密文的第一个曲线点。
	CipherA Point
	// CipherB 是密文的第二个曲线点。
	CipherB Point
	// NumOptions 是提案的选项数。
	NumOptions fr.Element
}

// BuildPublicInputs 从领域层的公开输入声明构造电路公开输入。
//
// 输入里的每个字段都来自服务端自己能拿到的材料：请求里的密文与
// nullifier、提案的选项数、计票会话里的公钥。客户端不提交公开输入，
// 因此一份 proof 只能用于它被生成的那一份密文。
func BuildPublicInputs(statement entity.BallotStatement) (PublicInputs, error) {
	if statement.NumOptions <= 0 || statement.NumOptions > MaxOptions {
		return PublicInputs{}, fmt.Errorf("invalid option count: %d", statement.NumOptions)
	}
	if len(statement.Nullifier) != ScalarSize {
		return PublicInputs{}, fmt.Errorf("invalid nullifier length: %d", len(statement.Nullifier))
	}
	if !isCanonicalScalar(statement.Nullifier) {
		return PublicInputs{}, fmt.Errorf("nullifier encoding is not canonical")
	}
	key, err := parsePoint(statement.ElectionPublicKey)
	if err != nil {
		return PublicInputs{}, fmt.Errorf("decode election public key: %w", err)
	}
	a, b, err := parseCiphertext(statement.Ciphertext)
	if err != nil {
		return PublicInputs{}, err
	}

	var nullifier, numOptions fr.Element
	nullifier.SetBytes(statement.Nullifier)
	numOptions.SetUint64(uint64(statement.NumOptions))

	return PublicInputs{
		ProposalID:  DeriveProposalField(statement.ProposalID),
		Nullifier:   nullifier,
		ElectionKey: key,
		CipherA:     a,
		CipherB:     b,
		NumOptions:  numOptions,
	}, nil
}
