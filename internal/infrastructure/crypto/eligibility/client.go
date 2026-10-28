package eligibility

import (
	"fmt"

	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// ClientEligibilityRequest 是客户端生成资格证明所需的输入。
type ClientEligibilityRequest struct {
	// MemberPackage 是成员自己的资格包（秘密 + 叶子 + 路径）。
	MemberPackage MemberPackage
	// ProposalID 是提案 ID 原文。
	ProposalID string
	// Challenge 是服务端在本次领取会话里给出的挑战。
	Challenge string
	// Root 是服务端配置的名单树根 —— **以服务端为准**，不是包里那个。
	Root string
}

// ClientEligibilityResult 是资格证明的结果。
type ClientEligibilityResult struct {
	// Proof 是 Groth16 证明。
	Proof []byte
	// QualificationID 是去重键（十六进制）。
	QualificationID string
}

// ProveEligibility 在客户端本地生成一份资格证明。
//
// 它做三件事：
//
//  1. 由秘密重算叶子，与包里给的叶子比对 —— 包被改过就当场报错；
//  2. 由叶子与提案推出去重键；
//  3. 用服务端的 root 出证明。
//
// **root 用服务端的，不包里的**：服务端只认自己配置的那个 root，包里的
// root 只是成员自查用的。两边不一致时，客户端直接报错（而不是出一份
// 注定验不过的证明）。
func (p *Prover) ProveEligibility(request ClientEligibilityRequest) (*ClientEligibilityResult, error) {
	pack := request.MemberPackage
	if pack.Depth != p.depth {
		return nil, fmt.Errorf("%w: member package depth %d does not match circuit depth %d",
			ErrInvalidProof, pack.Depth, p.depth)
	}
	if len(pack.Path) != p.depth {
		return nil, fmt.Errorf("%w: member package path must have %d elements, got %d",
			ErrInvalidProof, p.depth, len(pack.Path))
	}
	if request.Root == "" {
		return nil, fmt.Errorf("%w: root is required", ErrInvalidProof)
	}
	if pack.Root != "" && pack.Root != request.Root {
		return nil, fmt.Errorf(
			"%w: the member package was issued for a different eligibility list",
			ErrInvalidProof,
		)
	}

	secretBytes, err := decodeHex(pack.Secret)
	if err != nil {
		return nil, fmt.Errorf("%w: decode secret: %v", ErrInvalidProof, err)
	}
	secret, err := SecretToField(secretBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: secret: %v", ErrInvalidProof, err)
	}

	leaf, err := DeriveLeaf(secretBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: derive leaf: %v", ErrInvalidProof, err)
	}
	packLeaf, err := DecodeField(pack.Leaf)
	if err != nil {
		return nil, fmt.Errorf("%w: decode leaf: %v", ErrInvalidProof, err)
	}
	if !leaf.Equal(&packLeaf) {
		return nil, fmt.Errorf("%w: the member package leaf does not match its secret",
			ErrInvalidProof)
	}

	path := make([]fr.Element, p.depth)
	for i, raw := range pack.Path {
		element, err := DecodeField(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: decode path element %d: %v", ErrInvalidProof, i, err)
		}
		path[i] = element
	}

	proposalField := ballot.DeriveProposalField(request.ProposalID)
	qualificationID := DeriveQualificationID(leaf, proposalField)

	statement, err := BuildStatement(
		request.Root,
		request.ProposalID,
		request.Challenge,
		EncodeField(qualificationID),
	)
	if err != nil {
		return nil, fmt.Errorf("%w: build statement: %v", ErrInvalidProof, err)
	}

	proof, err := p.Prove(statement, secret, pack.Index, path)
	if err != nil {
		return nil, err
	}
	return &ClientEligibilityResult{
		Proof:           proof,
		QualificationID: EncodeField(qualificationID),
	}, nil
}
