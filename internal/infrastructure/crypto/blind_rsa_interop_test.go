package crypto

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"testing"

	"github.com/cloudflare/circl/blindsign/blindrsa"
	"github.com/stretchr/testify/require"
)

// testIssuerPrivateKeyPem 是**仅用于测试**的固定 RSA 密钥。
//
// 它只出现在这个跨语言向量测试里，不参与任何生产路径：真实签发密钥由
// `crypto.issuer_key_path` 管理，落在服务端。
const testIssuerPrivateKeyPem = `-----BEGIN RSA TEST KEY-----
MIIEogIBAAKCAQEAqPagrnHfQayqGS2kYeHhJQPdV+I2Rv0i+mEw5/uCGJXxUIp7
0g1eFzc2VSqbLh0nWNl3d+qnNQ5mgjlXT7ARJqUG7rR3NFHvZOlIsbD19jZcIOA3
3Fq4JOMqQld3z6jkIF6b4WhP4QPRPGKCZs9BIF2NhJrJd/Lw9AD9slR8OAPgP15R
KzInW4B522klBjPdEMjj3XfoJG9VC9I2pPrRU12+BhfFt7wkwc25Sy+GXnfu4z8D
/lk3CVVfHBlpzFVdoIX1PiZ8BYqsSVwYDiWDl03uZNX5CUwCVQTUVF5bplpSx9v1
vMrFLAUkTY8JuRllkOgK0KDs/fDPK+NTJadFqQIDAQABAoIBABCbFJWweVmyB5Gr
67D3CzKhG5rsj5sQe+SRssXJ8TJLYkkhIyQ0zh+H/J4ILvIsSVaxIM23UTGktaxs
yEj7q+PZakIBmIma1+VaDBvyoWnOU+WjQ7ozbE+rx9Z3VD4jgjAwPC++GcOqpWDC
6cAC9IqixyrWL6DLTqUch9KD/SVgCz/fVBuqqpM9gZJcU9Ua5q4SeJ9R329cRaUK
NENhx2ekSs2+A/bLl7Ad7DfR943WJ3XrPSszsiy0ShUb+wNbXdyHuGiWKpGp8zjq
U8bMqlGh5c1jobpGWcF0VozF2Gq6OseGNwSeEqHV1psTSP+5CWc/HONAzbP9aZ5T
GSiIBWUCgYEA1ZQL+IxlmFFwJ0ysElO75wa8d9N5Qld9gwWGffKhhujrd6TXDbUF
cROzX+KoFeF3YY8shYmLmzXhVnpgDhigKvZE4ohKcwNuKXRskeDtWxqZt9TSqFf7
UU6BB4Ud0lmNI0RlzVodnHJldHUVN5rCO4Mmy4Jv52SORMrG6IzgdlMCgYEAyoYE
fFoCPsHiBs7kQRpBHvpiAYecNUWpuNp2b4q4kZ9wY6GRaeDynmNbeobEl7mLxLDZ
kzm/NLXnbEwcKQ2OmLIPl6+tFvczVFXEsNuvHIMN+w1kcsIVqxxzWGQVNNF8UscH
Z9NkBIVM1wm5h5h6BBXsAgsS5Yhot3YHQK4c3JMCgYAE+EXzCalPiw2WDIWwg3xK
2vBFxYj7zzdfdiB0C9JQZlkcVpyeeAjhYbj50TdiEvYJMiCyFspF2VIeBgj4KwCH
H2Xo84zv8jN9CXWBS3+acBsFmREx06JIeRAtkZ5pB6ZuHKhVx/QKcOsFc7gDrDk6
00ly0N0zwDZDSYipXqXWewKBgE69HDFcrVLI/p+3AHdvRbJNlp0RpGHx27LK/REQ
1gGIA6I4xthrZOi/qGlSzExRD05Bk9Xxta1Aq+EnFiu/4ZjsYbOP0bQEGCjJz9cT
Eln7egwJgOw9GT+VqCLNhpEqgTONvgtX3KMFUIKqfT+CwK2l+EQzGf2zs0Lg2KEF
EKORAoGAK78RJQG7Ng1t77Fh5X/zFVKLL9RHGde0rD9y+vkmGOXuO1xM/R5Jpuh8
Vz4P0vedKm5Pmy1cNmWtzcBal1Di9QqqxLijNNgkxiIIkknB8wHYL0WGusQtreoD
15zY8g8vf5OIAtkxKW9RUhOqdW1zAm/Y3b6lHaaOSuE8nSEHTC4=
-----END RSA TEST KEY-----
`

// 跨语言向量：浏览器端（web/src/lib/blind-rsa.test.ts）断言的是同一组值。
//
// 盲签名分两侧：客户端负责盲化与解盲，签发方只对盲化消息做一次 RSA 私钥
// 运算。两侧对同一条消息算出的盲化消息必须逐字节一致，否则解出来的签名
// 验不过 —— 而且**不会报错**，只是验签失败。
const (
	katBlindedMessage = "63a28a939dd013af69c53a512b8d3dcb45de1da623d1ba587a8ee4a781f6a96f01b0ca8e99ddfc6a231b28bc635f3c8f290c472be0a34d004d2f341250673eb92d94afddb44ea7c42d2e8d7507df8250b9e8892f6eba2098923bb6b675e875cda012feb6f6cfe980b3af482083c261efcf214c8249230581f9759087268ec89bda61c85feafadd09c443aed18d567e79f8723a048358b1ab020a791ebe49e925b663372a95ed4b4faf32d9dcae67b56b11f42a03410884688f7f44fcf9b535a52b8ebf6b2fdfd99b47b54de4738169b3a787928ea4c8a309bf73fbdc98830e1c5989d020c4141d083bba7b963db5fad166ec7d5fde38a790e8470583712c2075"
	katBlindSignature = "7f01d9c866f6fac88a8ddbdf415509ed39c5d331f72ef615afc5d156b7349045ec68e14138e80ad755099e7994176e6e825edfae7c5a78584f177156efb163465e519de4cb24eeda42c5ef51e3e47a19e717b030f75e2f1d6742d7f01e2d250269d33ec41d64306083f8a1f1d33d04d0b31a772cef9b36937797cd4c71e9b50ee97da97b3faca81d830f725a595b94d93abbe5506ea45c28f81ffd78fa0022baa20e5073834e81351563384252d5b13ee9df0be15b58987758412df23b857bd01c83a626603d0881310a8325b73c6a2ec2d82c41c5af317501fdcb53ccae5d7feeca3c960ca7b09bcc3d1314566ac8df3a285776267f1cc1f852dd78cde3b337"
	katSignature      = "72b5aabb439a72e53ceb7d321ef452cb53ba9c9345fa7fdd2dadfa9637ded064ec67626c903d00a23ac91a75770c656919069a4e150060a906300eed61fbe7232b6a7349537e26460165460757615b5a0b5d69629924e70ef982e8c348ad25e4d8ff8b7e69a58f737957bce4c377ea1ad262c1fd1cdaf4aa8b4e835649c6c8b679323e93cc962409fedc96383d185642744e7bc689a48fe5247ff4c33df5ead8a60d0cbc891fa0ca9474d565a30e7c1408316254b7a46f22fd5f390df747f4c6bc0c76047569b703e08741c42f2b9da1510033507ee8dff3fda99e1cb993796c27610811eb6299f0955e4ff1ca9f79fb37ec044e8be489816b085cc5787f43d5"
)

func loadTestIssuerKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()

	block, _ := pem.Decode([]byte(testIssuerPrivateKeyPem))
	require.NotNil(t, block, "测试密钥 PEM 解析失败")
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	require.NoError(t, err)
	return key
}

// TestKAT_BlindRsaClientSide 锁死浏览器端盲签名 client 侧与 CIRCL 的一致性。
//
// 固定盐与固定盲化因子：两边的随机源因此可控，盲化消息可以逐字节比对。
// CIRCL 的 Blind 先读盐，再读盲化因子（`crypto/rand.Int` 读 256 字节并把
// 最高字节 & 0x7f）—— 所以盲化因子的最高字节必须高位为 0。
func TestKAT_BlindRsaClientSide(t *testing.T) {
	t.Parallel()

	privateKey := loadTestIssuerKey(t)
	client, err := blindrsa.NewClient(blindrsa.SHA384PSSDeterministic, &privateKey.PublicKey)
	require.NoError(t, err)
	signer := blindrsa.NewSigner(privateKey)

	// 凭证消息与 crypto 包的 CredentialMessage 同口径。
	message := CredentialMessage("u-blind-1", []byte("one-time-public-key"))
	require.Equal(
		t,
		"c5518e49bf357e08eace129b9eb4104bb8620264b6c903cdd12a667fefa4e297",
		hex.EncodeToString(message),
	)

	salt := make([]byte, 48)
	for i := range salt {
		salt[i] = byte(i + 1)
	}
	blindingBytes := make([]byte, 256)
	for i := range blindingBytes {
		blindingBytes[i] = byte((i*7 + 3) % 251)
	}
	blindingBytes[0] = 0x01
	blindingBytes[255] |= 1
	blindingFactor := new(big.Int).SetBytes(blindingBytes)
	require.Less(t, blindingFactor.Cmp(privateKey.PublicKey.N), 0)
	require.Equal(
		t,
		big.NewInt(1),
		new(big.Int).GCD(nil, nil, blindingFactor, privateKey.PublicKey.N),
		"盲化因子必须与模数互素",
	)

	reader := bytes.NewReader(append(append([]byte{}, salt...), blindingBytes...))
	prepared, err := client.Prepare(rand.Reader, message)
	require.NoError(t, err)
	blinded, state, err := client.Blind(reader, prepared)
	require.NoError(t, err)
	require.Equal(t, katBlindedMessage, hex.EncodeToString(blinded))

	blindSignature, err := signer.BlindSign(blinded)
	require.NoError(t, err)
	require.Equal(t, katBlindSignature, hex.EncodeToString(blindSignature))

	signature, err := client.Finalize(state, blindSignature)
	require.NoError(t, err)
	require.Equal(t, katSignature, hex.EncodeToString(signature))
}

// TestKAT_BlindRsaVariantName 锁死盲签名变体在**线路上的名字**。
//
// 服务端把签发方公钥连同变体名一起发给客户端（claim 响应的 `variant` 字段），
// 客户端据此确认自己实现的变体与服务端一致：变体不同，盲化结果就不同，
// 而且不会当场报错，只是最后验签失败。
//
// 线路名是 CIRCL `blindsign/blindrsa` 的 `Variant.String()`，
// **不是** Go 侧的枚举标识符 `SHA384PSSDeterministic` —— 写错时门户的
// 匿名领取与管理端签发都会直接失败。
//
// 跨语言锁定：浏览器端断言的是同一个字符串
// （`web/src/lib/credential.test.ts` 的 `变体名与服务端线路上的名字一致`）。
func TestKAT_BlindRsaVariantName(t *testing.T) {
	t.Parallel()

	require.Equal(t, "RSABSSA-SHA384-PSS-Deterministic", blindrsa.SHA384PSSDeterministic.String())
}
