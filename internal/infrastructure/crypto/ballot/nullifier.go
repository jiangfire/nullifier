package ballot

import (
	"crypto/sha256"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
)

// mimcConstants 缓存 MiMC 的轮常量，避免每次推导都重新构造。
var mimcConstants = mimc.GetConstants()

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
	return mimcHash(nullifierSecret, proposalField)
}

// mimcHash 是与电路侧 std/hash/mimc 完全一致的 MiMC 实现。
//
// 轮函数是 x = (x + h + c)^5，外层是 Miyaguchi–Preneel：
// h = h + E(x) + x。两侧必须逐位一致，电路里的 nullifier 约束才有意义，
// 因此这份实现与 gnark 的 std/hash/mimc 保持同一套常量与同一套轮次。
func mimcHash(elements ...fr.Element) fr.Element {
	var h fr.Element
	for _, element := range elements {
		m := element
		for i := range mimcConstants {
			var constant, sum fr.Element
			constant.SetBigInt(&mimcConstants[i])
			sum.Add(&m, &h)
			sum.Add(&sum, &constant)

			var square, fourth fr.Element
			square.Square(&sum)
			fourth.Square(&square)
			m.Mul(&fourth, &sum)
		}
		m.Add(&m, &h)
		h.Add(&h, &m)
		h.Add(&h, &element)
	}
	return h
}
