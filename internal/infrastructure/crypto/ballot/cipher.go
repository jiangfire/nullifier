package ballot

import (
	"errors"
	"fmt"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

const (
	// CipherVersion 是选票密文的格式版本。
	//
	// v3 与 v0.1.0 的 v2 密文不兼容：v2 是 ed25519 上的 DH 混合封装，
	// v3 是 BN254 twisted Edwards 上的指数 ElGamal。旧密文解不开，也不
	// 应该再用于真实投票。
	CipherVersion byte = 3
	// CipherSize 是 v3 密文的字节长度：1 字节版本号 + 两个压缩点。
	CipherSize = 1 + 2*PointSize
)

// ErrInvalidCiphertext 表示选票密文的结构、版本或曲线元素不合法。
var ErrInvalidCiphertext = errors.New("invalid ballot ciphertext")

// EncryptBallot 用提案的计票公钥加密选项下标，返回密文与一次性随机数。
//
// 密文是 (a, b) = (r·G, m·G + r·Y)：m 是选项下标，r 是随机数，Y 是计票
// 公钥。服务端只有密文时无法还原 m —— 要解出 m 需要门限私钥 X。
func EncryptBallot(publicKey []byte, optionIndex uint64) ([]byte, fr.Element, error) {
	if optionIndex >= MaxOptions {
		return nil, fr.Element{}, fmt.Errorf("%w: option index out of range: %d", ErrInvalidCiphertext, optionIndex)
	}
	key, err := parsePoint(publicKey)
	if err != nil {
		return nil, fr.Element{}, fmt.Errorf("%w: decode election public key: %v", ErrInvalidCiphertext, err)
	}
	nonce, err := randomScalar()
	if err != nil {
		return nil, fr.Element{}, err
	}
	base := basePoint()
	var option fr.Element
	option.SetUint64(optionIndex)

	a := base.scalarMul(nonce)
	b := base.scalarMul(option).add(key.scalarMul(nonce))

	out := make([]byte, 0, CipherSize)
	out = append(out, CipherVersion)
	out = append(out, a.marshal()...)
	out = append(out, b.marshal()...)
	return out, nonce, nil
}

// DecryptBallotOption 用提案的计票私钥还原选项下标。
//
// 解密是 b - X·a = m·G，再在提案的选项集合里比对出 m。选项数很小，
// 逐个比对即可。
func DecryptBallotOption(privateKey []byte, ciphertext []byte, numOptions int) (uint64, error) {
	if numOptions <= 0 || numOptions > MaxOptions {
		return 0, fmt.Errorf("%w: invalid option count: %d", ErrInvalidCiphertext, numOptions)
	}
	if len(privateKey) != ScalarSize {
		return 0, fmt.Errorf("%w: invalid private key length: %d", ErrInvalidCiphertext, len(privateKey))
	}
	var secret fr.Element
	secret.SetBytes(privateKey)

	a, b, err := parseCiphertext(ciphertext)
	if err != nil {
		return 0, err
	}
	plain := b.sub(a.scalarMul(secret))

	base := basePoint()
	for index := 0; index < numOptions; index++ {
		var candidate fr.Element
		candidate.SetUint64(uint64(index))
		if base.scalarMul(candidate).equal(plain) {
			return uint64(index), nil
		}
	}
	return 0, fmt.Errorf("%w: decrypted ballot does not match any option", ErrInvalidCiphertext)
}

// ValidateBallotCiphertext 校验密文的结构与曲线元素。
func ValidateBallotCiphertext(ciphertext []byte) error {
	_, _, err := parseCiphertext(ciphertext)
	return err
}

// parseCiphertext 解析密文里的两个曲线点。
func parseCiphertext(ciphertext []byte) (Point, Point, error) {
	if len(ciphertext) != CipherSize {
		return Point{}, Point{}, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidCiphertext, CipherSize, len(ciphertext))
	}
	if ciphertext[0] != CipherVersion {
		return Point{}, Point{}, fmt.Errorf("%w: unsupported version: %d", ErrInvalidCiphertext, ciphertext[0])
	}
	a, err := parsePoint(ciphertext[1 : 1+PointSize])
	if err != nil {
		return Point{}, Point{}, fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	b, err := parsePoint(ciphertext[1+PointSize:])
	if err != nil {
		return Point{}, Point{}, fmt.Errorf("%w: %v", ErrInvalidCiphertext, err)
	}
	return a, b, nil
}
