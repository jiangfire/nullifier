package ballot

import (
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// EncodeScalarHex 把标量编码成 64 位十六进制字符串（大端）。
func EncodeScalarHex(scalar fr.Element) string {
	raw := scalar.Bytes()
	return hex.EncodeToString(raw[:])
}

// GenerateNullifierSecret 采样一个 nullifier 秘密。
//
// 返回的字节是规范编码的标量：可以直接编码成 nullifier，也可以作为
// 电路里的 nullifier 秘密见证。
func GenerateNullifierSecret() ([]byte, error) {
	secret, err := randomScalar()
	if err != nil {
		return nil, err
	}
	return scalarBytes(secret), nil
}

// ParseScalarHex 解析十六进制编码的域元素。
//
// 与 SetBytes 的区别：SetBytes 会把超范围的输入按模 r 归约，这里直接
// 拒绝非规范编码，避免同一个 nullifier 出现多种字节表示。
func ParseScalarHex(value string) (fr.Element, error) {
	if len(value) != ScalarSize*2 {
		return fr.Element{}, fmt.Errorf("invalid scalar encoding length: %d", len(value))
	}
	raw, err := hex.DecodeString(value)
	if err != nil {
		return fr.Element{}, fmt.Errorf("decode scalar: %w", err)
	}
	if !isCanonicalScalar(raw) {
		return fr.Element{}, fmt.Errorf("scalar encoding is not canonical")
	}
	var scalar fr.Element
	scalar.SetBytes(raw)
	return scalar, nil
}

// isCanonicalScalar 判断字节串是否是规范（小于模数）的域元素编码。
func isCanonicalScalar(raw []byte) bool {
	if len(raw) != ScalarSize {
		return false
	}
	return new(big.Int).SetBytes(raw).Cmp(frModulus()) < 0
}
