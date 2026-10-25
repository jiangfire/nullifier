package ballot

import (
	"encoding/hex"
	"testing"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/stretchr/testify/require"
)

// TestUncompressPoint 覆盖未压缩点的导出：客户端用它本地加密。
func TestUncompressPoint(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)

	uncompressed, err := UncompressPoint(keys.Public)
	require.NoError(t, err)
	require.Len(t, uncompressed, 2*PointSize)

	// 未压缩形式必须与压缩形式指向同一个群元素。
	parsed, err := parseUncompressedPoint(uncompressed)
	require.NoError(t, err)
	compressed, err := parsePoint(keys.Public)
	require.NoError(t, err)
	require.True(t, parsed.equal(compressed))

	// 长度不对、点不合法都要被拒。
	_, err = UncompressPoint(keys.Public[:PointSize-1])
	require.Error(t, err)
	broken := append([]byte(nil), uncompressed...)
	broken[0] ^= 0x01
	_, err = UncompressPoint(broken)
	require.Error(t, err)
}

// TestCompressPoint 覆盖未压缩 → 压缩的转换：客户端用它与服务端拼出同一份
// 公开输入。
func TestCompressPoint(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := UncompressPoint(keys.Public)
	require.NoError(t, err)

	compressed, err := CompressPoint(uncompressed)
	require.NoError(t, err)
	require.Len(t, compressed, PointSize)
	// 往返必须回到同一个字节：客户端与服务端的公开输入要逐字节一致。
	require.Equal(t, keys.Public, compressed)

	// 长度不对、点不合法都要被拒。
	_, err = CompressPoint(uncompressed[:2*PointSize-1])
	require.Error(t, err)
	broken := append([]byte(nil), uncompressed...)
	broken[0] ^= 0x01
	_, err = CompressPoint(broken)
	require.Error(t, err)
}

// TestEncryptBallotWithUncompressedKey 覆盖客户端加密入口。
func TestEncryptBallotWithUncompressedKey(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := UncompressPoint(keys.Public)
	require.NoError(t, err)

	ciphertext, nonce, err := EncryptBallotWithUncompressedKey(uncompressed, 3)
	require.NoError(t, err)
	require.Len(t, ciphertext, CipherSize)
	require.NoError(t, ValidateBallotCiphertext(ciphertext))

	index, err := DecryptBallotOption(keys.Private, ciphertext, 4)
	require.NoError(t, err)
	require.Equal(t, uint64(3), index)

	// 随机数必须是一次性的：两次加密的密文不同。
	other, _, err := EncryptBallotWithUncompressedKey(uncompressed, 3)
	require.NoError(t, err)
	require.NotEqual(t, hex.EncodeToString(ciphertext), hex.EncodeToString(other))
	require.NotEqual(t, nonce.String(), other[0])
}

// TestEncryptBallotWithUncompressedKey_Invalid 覆盖非法输入。
func TestEncryptBallotWithUncompressedKey_Invalid(t *testing.T) {
	keys, err := GenerateKeyPair()
	require.NoError(t, err)
	uncompressed, err := UncompressPoint(keys.Public)
	require.NoError(t, err)

	// 长度不对。
	_, _, err = EncryptBallotWithUncompressedKey(uncompressed[:10], 0)
	require.Error(t, err)

	// 点不在曲线上。
	broken := append([]byte(nil), uncompressed...)
	broken[0] ^= 0x01
	_, _, err = EncryptBallotWithUncompressedKey(broken, 0)
	require.Error(t, err)

	// 选项下标越界。
	_, _, err = EncryptBallotWithUncompressedKey(uncompressed, MaxOptions)
	require.Error(t, err)
}

// TestParseUncompressedPoint_NotInSubGroup 确认子群检查真的在跑。
//
// (0, -1) 是这条曲线上的二阶点：在曲线上，但**不在**素数阶子群里。小阶点
// 会让解密方泄漏私钥低位比特，所以外部来的点必须过这一关。
func TestParseUncompressedPoint_NotInSubGroup(t *testing.T) {
	var zero fr.Element
	zero.SetZero()
	var minusOne fr.Element
	minusOne.SetOne()
	minusOne.Neg(&minusOne)

	raw := make([]byte, 0, 2*PointSize)
	x := zero.Bytes()
	y := minusOne.Bytes()
	raw = append(raw, x[:]...)
	raw = append(raw, y[:]...)

	point := Point{X: zero, Y: minusOne}
	require.True(t, point.isOnCurve())
	require.False(t, point.isInSubGroup())

	_, err := parseUncompressedPoint(raw)
	require.Error(t, err)
	require.ErrorContains(t, err, "subgroup")
}
