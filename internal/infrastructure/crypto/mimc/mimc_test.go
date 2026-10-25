package mimc

import (
	"encoding/hex"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// MiMC 轮常量由「Keccak-256 链」派生，浏览器端必须用同一套常量。
//
// 这里把首尾两个常量钉住：常量表一旦与浏览器端错开，nullifier 与资格叶子
// 就会两边算出不同的值，而电路只会在验签时告诉你「不对」。
//
// 跨语言的对端断言：web/src/lib/mimc.test.ts。
func TestKAT_MimcConstants(t *testing.T) {
	t.Parallel()

	constants := Constants()
	require.Len(t, constants, 91)

	first := fr.Element{}
	first.SetBigInt(&constants[0])
	require.Equal(
		t,
		"00808370c37267481fb91b077899955706f209e5e0762dac2c79ba1e7a91b018",
		encodeScalarHex(first),
	)

	last := fr.Element{}
	last.SetBigInt(&constants[len(constants)-1])
	require.Equal(
		t,
		"0ad024352c40ea93df089e7950a8ef31949ca31185ea5554e507f1f5b71a82a8",
		encodeScalarHex(last),
	)
}

// encodeScalarHex 把标量编码成 64 位十六进制字符串（大端）。
func encodeScalarHex(scalar fr.Element) string {
	raw := scalar.Bytes()
	return hex.EncodeToString(raw[:])
}

// TestKAT_MimcHash 把哈希输出本身钉住。
//
// 常量表被钉住了还不够：轮函数或轮次一旦被动过，常量表可能不变而输出全变。
// 这份向量钉的是「选票 nullifier」与「资格叶子 / 去重键」共同的底层哈希 ——
// 它变了，两套电路的约束都不会再成立。
func TestKAT_MimcHash(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		elements []uint64
		want     string
	}{
		{
			name:     "empty",
			elements: nil,
			want:     "0000000000000000000000000000000000000000000000000000000000000000",
		},
		{
			name:     "one element",
			elements: []uint64{1},
			want:     "051f81ae0ab58ceaac24eb32499c26bfd1bf96bd3d6055450c89248011a32483",
		},
		{
			name:     "two elements",
			elements: []uint64{1, 2},
			want:     "18f4966ab2503ee8faa7751bbab227b0bfe9c9922a9e84716a5d5b002214b32a",
		},
		{
			name:     "three elements",
			elements: []uint64{3, 4, 5},
			want:     "1fa057d1858b13c0fbe5487d335b17db62575008eeea896866b982c253cdffa3",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			inputs := make([]fr.Element, 0, len(tc.elements))
			for _, v := range tc.elements {
				var e fr.Element
				e.SetUint64(v)
				inputs = append(inputs, e)
			}

			require.Equal(t, tc.want, encodeScalarHex(Hash(inputs...)))
		})
	}
}

// TestHash_DistinctInputsDistinctOutputs 断言哈希不是退化成常数。
func TestHash_DistinctInputsDistinctOutputs(t *testing.T) {
	t.Parallel()

	var one, two fr.Element
	one.SetUint64(1)
	two.SetUint64(2)

	require.NotEqual(t, encodeScalarHex(Hash(one)), encodeScalarHex(Hash(two)))
	require.NotEqual(t, encodeScalarHex(Hash(one)), encodeScalarHex(Hash(one, two)))
	require.NotEqual(t, encodeScalarHex(Hash(one, two)), encodeScalarHex(Hash(two, one)))
}
