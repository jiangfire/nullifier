package ballot

import (
	"errors"
	"fmt"
	"sort"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// shareIndexSize 是门限份额里下标的字节长度（大端 uint32）。
const shareIndexSize = 4

// KeyPair 是提案的计票密钥对：私钥是 BN254 标量域里的标量，公钥是
// twisted Edwards 曲线上的压缩点。
type KeyPair struct {
	// Private 是 32 字节标量（大端）。
	Private []byte
	// Public 是 32 字节压缩点。
	Public []byte
}

// GenerateKeyPair 生成计票密钥对。
func GenerateKeyPair() (*KeyPair, error) {
	secret, err := randomScalar()
	if err != nil {
		return nil, err
	}
	public := basePoint().scalarMul(secret)
	return &KeyPair{
		Private: scalarBytes(secret),
		Public:  public.marshal(),
	}, nil
}

// SplitSecret 把私钥拆成 threshold-of-trustees 的 Shamir 份额。
//
// 份额的插值在 BN254 标量域上进行：与选票密文所在的群是同一个域，
// 门限解密与密文格式因此不需要额外转换。
func SplitSecret(secret []byte, threshold, trustees int) ([][]byte, error) {
	if threshold <= 0 {
		return nil, errors.New("threshold must be positive")
	}
	if trustees < threshold {
		return nil, errors.New("trustees must be >= threshold")
	}
	if len(secret) != ScalarSize {
		return nil, fmt.Errorf("invalid secret length: %d", len(secret))
	}
	var root fr.Element
	root.SetBytes(secret)

	coefficients := make([]fr.Element, threshold)
	coefficients[0] = root
	for i := 1; i < threshold; i++ {
		random, err := randomScalar()
		if err != nil {
			return nil, err
		}
		coefficients[i] = random
	}

	shares := make([][]byte, 0, trustees)
	for i := 1; i <= trustees; i++ {
		value := evaluatePolynomial(coefficients, uint32(i))
		shares = append(shares, encodeShare(uint32(i), value))
	}
	return shares, nil
}

// RecoverSecret 用门限份额还原私钥。
func RecoverSecret(shares [][]byte, threshold int) ([]byte, error) {
	if threshold <= 0 {
		return nil, errors.New("threshold must be positive")
	}
	if len(shares) < threshold {
		return nil, errors.New("insufficient shares")
	}
	points := make([]sharePoint, 0, len(shares))
	for _, raw := range shares {
		index, value, err := decodeShare(raw)
		if err != nil {
			return nil, err
		}
		points = append(points, sharePoint{Index: index, Value: value})
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Index < points[j].Index })
	for i := 1; i < len(points); i++ {
		if points[i].Index == points[i-1].Index {
			return nil, errors.New("duplicate share index")
		}
	}
	secret, err := interpolateAtZero(points[:threshold])
	if err != nil {
		return nil, err
	}
	return scalarBytes(secret), nil
}

// sharePoint 是插值时用的一个份额。
type sharePoint struct {
	Index uint32
	Value fr.Element
}

// evaluatePolynomial 用 Horner 法求多项式在 x 处的值。
func evaluatePolynomial(coefficients []fr.Element, x uint32) fr.Element {
	var result fr.Element
	var xElement fr.Element
	xElement.SetUint64(uint64(x))
	for i := len(coefficients) - 1; i >= 0; i-- {
		result.Mul(&result, &xElement)
		result.Add(&result, &coefficients[i])
	}
	return result
}

// interpolateAtZero 用拉格朗日插值还原 f(0)。
func interpolateAtZero(points []sharePoint) (fr.Element, error) {
	var result fr.Element
	for i := range points {
		var numerator, denominator fr.Element
		numerator.SetOne()
		denominator.SetOne()
		var xi fr.Element
		xi.SetUint64(uint64(points[i].Index))
		for j := range points {
			if i == j {
				continue
			}
			var xj, difference, negative fr.Element
			xj.SetUint64(uint64(points[j].Index))
			difference.Sub(&xi, &xj)
			negative.Neg(&xj)
			numerator.Mul(&numerator, &negative)
			denominator.Mul(&denominator, &difference)
		}
		if denominator.IsZero() {
			return fr.Element{}, errors.New("share indexes are not distinct")
		}
		var inverse fr.Element
		inverse.Inverse(&denominator)
		var term fr.Element
		term.Mul(&points[i].Value, &numerator)
		term.Mul(&term, &inverse)
		result.Add(&result, &term)
	}
	return result, nil
}

// encodeShare 序列化份额：4 字节下标 + 32 字节取值。
func encodeShare(index uint32, value fr.Element) []byte {
	out := make([]byte, shareIndexSize+ScalarSize)
	out[0] = byte(index >> 24)
	out[1] = byte(index >> 16)
	out[2] = byte(index >> 8)
	out[3] = byte(index)
	valueBytes := value.Bytes()
	copy(out[shareIndexSize:], valueBytes[:])
	return out
}

// decodeShare 解析份额。
func decodeShare(raw []byte) (uint32, fr.Element, error) {
	if len(raw) != shareIndexSize+ScalarSize {
		return 0, fr.Element{}, fmt.Errorf("invalid share length: %d", len(raw))
	}
	index := uint32(raw[0])<<24 | uint32(raw[1])<<16 | uint32(raw[2])<<8 | uint32(raw[3])
	var value fr.Element
	value.SetBytes(raw[shareIndexSize:])
	return index, value, nil
}

// scalarBytes 返回标量的规范字节表示。
func scalarBytes(s fr.Element) []byte {
	raw := s.Bytes()
	return append([]byte(nil), raw[:]...)
}
