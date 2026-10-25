// Package mimc 提供与 gnark `std/hash/mimc` 逐位一致的 MiMC 哈希。
//
// 它被两处电路共用：选票电路的 nullifier 约束（`ballot`）与资格证明电路的
// Merkle 承诺（`eligibility`）。**两处必须用同一份实现** —— 差一位，
// 客户端就出不了证明（而且不会报错，只是验签失败）。
//
// 轮函数是 x = (x + h + c)^5，外层是 Miyaguchi–Preneel：h = h + E(x) + x。
// 每个域元素是 32 字节（正好一个 BlockSize），因此「写 n 个元素」=「n 个
// chunk」，与 gnark-crypto 的 digest.checksum 行为一致。
package mimc

import (
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr/mimc"
)

// constants 缓存 MiMC 的轮常量，避免每次推导都重新构造。
var constants = mimc.GetConstants()

// Constants 返回 MiMC 的轮常量。
//
// 跨语言 KAT 用它把常量表钉住（浏览器端 `web/src/lib/mimc.test.ts` 断言的是
// 同一批值）：常量一旦与浏览器端错开，nullifier 与资格叶子就会两边算出不同
// 的值，而电路只会在验签时告诉你「不对」。
func Constants() []big.Int {
	return constants
}

// Hash 是与电路侧 std/hash/mimc 完全一致的 MiMC 实现。
//
// 轮函数是 x = (x + h + c)^5，外层是 Miyaguchi–Preneel：
// h = h + E(x) + x。两侧必须逐位一致，电路里的哈希约束才有意义，
// 因此这份实现与 gnark 的 std/hash/mimc 保持同一套常量与同一套轮次。
func Hash(elements ...fr.Element) fr.Element {
	var h fr.Element
	for _, element := range elements {
		m := element
		for i := range constants {
			var constant, sum fr.Element
			constant.SetBigInt(&constants[i])
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
