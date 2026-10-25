package ballot

import (
	"crypto/sha256"

	"anonvote/internal/infrastructure/crypto/mimc"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// DeriveProposalField 把提案 ID 映射成标量域元素。
//
// 用 SHA-256 而不是 MiMC 是有意的：提案 ID 是公开值，不需要在电路里复算；
// 电路只需要它在两侧得到同一个域元素即可。
func DeriveProposalField(proposalID string) fr.Element {
	sum := sha256.Sum256([]byte(proposalID))
	var field fr.Element
	field.SetBytes(sum[:])
	return field
}

// DeriveNullifier 用投票人的 nullifier 秘密与提案 ID 推导 nullifier。
//
// nullifier 必须是秘密的一向函数：它要公开在票表里做防重投，又不能让
// 服务端反推出秘密（否则就能伪造别人的 nullifier 抢占他们的投票机会）。
func DeriveNullifier(nullifierSecret fr.Element, proposalField fr.Element) fr.Element {
	return mimc.Hash(nullifierSecret, proposalField)
}
