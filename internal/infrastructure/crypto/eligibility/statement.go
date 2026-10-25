package eligibility

import (
	"fmt"

	"anonvote/internal/infrastructure/crypto/ballot"
	"anonvote/internal/infrastructure/crypto/mimc"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// Statement 是资格证明的公开输入。
//
// 五个元素，**定长** —— 公开输入的个数不随 Merkle 树深变化，服务端因此
// 不需要知道树的结构。
type Statement struct {
	// Root 是资格名单的树根。
	Root fr.Element
	// ProposalID 是提案 ID 映射出的域元素。
	ProposalID fr.Element
	// Challenge 是本次领取会话的一次性挑战。
	Challenge fr.Element
	// QualificationID 是去重键。
	QualificationID fr.Element
	// SessionBinding 把挑战钉在这份证明上（= MiMC(去重键, 挑战)）。
	SessionBinding fr.Element
}

// BuildStatement 由公开的字符串材料拼出公开输入。
//
// root 与 qualification_id 都是十六进制的域元素；challenge 同理。
// SessionBinding 由服务端自己算 —— 它知道去重键与挑战。
func BuildStatement(root, proposalID, challenge, qualificationID string) (Statement, error) {
	rootElement, err := DecodeField(root)
	if err != nil {
		return Statement{}, fmt.Errorf("decode root: %w", err)
	}
	challengeElement, err := DecodeField(challenge)
	if err != nil {
		return Statement{}, fmt.Errorf("decode challenge: %w", err)
	}
	qualificationElement, err := DecodeField(qualificationID)
	if err != nil {
		return Statement{}, fmt.Errorf("decode qualification id: %w", err)
	}
	return Statement{
		Root:            rootElement,
		ProposalID:      ballot.DeriveProposalField(proposalID),
		Challenge:       challengeElement,
		QualificationID: qualificationElement,
		SessionBinding:  DeriveSessionBinding(qualificationElement, challengeElement),
	}, nil
}

// DeriveSessionBinding 推导会话绑定值：MiMC(去重键, 挑战)。
func DeriveSessionBinding(qualificationID, challenge fr.Element) fr.Element {
	return mimc.Hash(qualificationID, challenge)
}
