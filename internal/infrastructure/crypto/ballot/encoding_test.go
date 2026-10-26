package ballot

import (
	"math/big"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestCanonicalScalarRange 钉住「规范域元素 = 值小于模数 r」这条判据，
// 以及它给「篡改 nullifier」带来的约束。
//
// 篡改 nullifier 时**不能碰最高字节**：BN254 的 r 顶字节是 0x30，顶字节恰好
// 是 0x30 的标量（在均匀分布里约占 1%）翻掉一位就越界，isCanonicalScalar 会
// 拒绝非规范编码 —— 那失败的就不是「验签没过」，而是「篡改方式不合法」。
// 改最低字节则安全：值只差 ±1，只有恰好撞上 r 才越界（概率 2⁻²⁵²）。
//
// 这条测试用确定性的取值把两种情形都摆出来，不靠概率。
func TestCanonicalScalarRange(t *testing.T) {
	modulus := frModulus()

	// 取 r-2：顶字节是 0x30（与真实 nullifier 撞上同一种情形），仍小于 r。
	// 之所以不是 r-1，是因为 r-1 的最低字节翻一位正好等于 r。
	justBelow := new(big.Int).Sub(modulus, big.NewInt(2)).Bytes()
	require.Len(t, justBelow, ScalarSize)
	require.Equal(t, byte(0x30), justBelow[0])
	require.True(t, isCanonicalScalar(justBelow))

	// 翻最高字节 → 顶字节变成 0x31，越过 r，不再是规范编码。
	topFlipped := append([]byte(nil), justBelow...)
	topFlipped[0] ^= 0x01
	require.False(t, isCanonicalScalar(topFlipped))

	// 翻最低字节 → 只差 ±1，仍在范围内，仍是规范编码。
	lowFlipped := append([]byte(nil), justBelow...)
	lowFlipped[len(lowFlipped)-1] ^= 0x01
	require.True(t, isCanonicalScalar(lowFlipped))
}
