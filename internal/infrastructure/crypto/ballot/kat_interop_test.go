package ballot

import (
	"encoding/hex"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// 跨语言一致性向量。
//
// 浏览器端（web/src/lib/ballot-cipher.ts）用同一套曲线参数与点编码生成选票
// 密文，服务端要用**完全相同**的编码解析它，否则密文解不开。这些值由
// 浏览器端实现生成后回填，两边都必须保持不变：改任何一边都要重新生成。
const (
	katPublicKeyUncompressed = "2c09976b87cbee617287650964760c30d77b01ab2a38d831b47661270ed3e01e1d" +
		"5a43dc73fe0493cce521cc92a4d34d4837214ce47871c587c567d2d0c72c8f"
	katPublicKeyCompressed = "8f2cc7d0d267c587c57178e44c2137484dd3a492cc21e5cc9304fe73dc435a9d"
	katBasePointCompressed = "8b7d2d877a253c4b7733e1b91f05e0fcedf96bd11c2e572549b2a0f703727925"
	katCiphertext          = "03e8f4c819f576c6516b8d79ae25528f86d68d8250e7ada580022fd42bab255" +
		"ea41c0390e198a87181b9419ecaf472b9c908de35d79bdb085d5f154033efffeb84"
	// katPrivateKey 对应上面这把计票公钥（标量 12345）。
	katPrivateKey = 12345
	// katOptionIndex 是浏览器端加密的选项下标。
	katOptionIndex = 2
)

// TestKAT_BrowserCiphertextIsDecryptable 是浏览器端与服务端之间的互操作证据。
//
// 密文由浏览器端生成（向量回填自 web/src/lib/ballot-cipher.test.ts），服务端
// 必须能校验结构、解出同一个选项下标。哪一边的点编码改了，这个用例就会红。
func TestKAT_BrowserCiphertextIsDecryptable(t *testing.T) {
	ciphertext, err := hex.DecodeString(katCiphertext)
	require.NoError(t, err)

	// 结构校验：版本号、点是否在曲线上、是否在素数阶子群里。
	require.NoError(t, ValidateBallotCiphertext(ciphertext))

	// 解密：还原出浏览器端选中的选项下标。
	var privateKey fr.Element
	privateKey.SetUint64(katPrivateKey)
	keyBytes := privateKey.Bytes()
	index, err := DecryptBallotOption(keyBytes[:], ciphertext, 3)
	require.NoError(t, err)
	require.Equal(t, uint64(katOptionIndex), index)
}

// TestKAT_PointEncodings 断言点编码与浏览器端一致。
func TestKAT_PointEncodings(t *testing.T) {
	require.Equal(t, katBasePointCompressed, hex.EncodeToString(basePoint().marshal()))

	var secret fr.Element
	secret.SetUint64(katPrivateKey)
	publicKey := basePoint().scalarMul(secret)
	require.Equal(t, katPublicKeyCompressed, hex.EncodeToString(publicKey.marshal()))

	// 未压缩形式：X 与 Y 各 32 字节大端，浏览器端用它做加密输入。
	affine := publicKey.affine()
	x := affine.X.Bytes()
	y := affine.Y.Bytes()
	require.Equal(t, katPublicKeyUncompressed, hex.EncodeToString(x[:])+hex.EncodeToString(y[:]))
}

// TestKAT_GoSideMatchesBrowserCiphertext 断言 Go 侧用同一组参数算出的密文
// 与浏览器端完全相同 —— 证明两侧的曲线运算与编码没有偏差。
func TestKAT_GoSideMatchesBrowserCiphertext(t *testing.T) {
	var secret fr.Element
	secret.SetUint64(katPrivateKey)
	publicKey := basePoint().scalarMul(secret)

	var nonce fr.Element
	nonce.SetUint64(987654321)
	var option fr.Element
	option.SetUint64(katOptionIndex)

	a := basePoint().scalarMul(nonce)
	b := basePoint().scalarMul(option).add(publicKey.scalarMul(nonce))
	ciphertext := append([]byte{CipherVersion}, a.marshal()...)
	ciphertext = append(ciphertext, b.marshal()...)

	require.Equal(t, katCiphertext, hex.EncodeToString(ciphertext))
}
