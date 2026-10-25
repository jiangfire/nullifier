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
