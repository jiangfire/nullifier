// Package ballot 实现 nullifier v0.2.0 的选票密码学栈。
//
// 选票密文（格式 v3）跑在 BN254 的 twisted Edwards 曲线上：选票下标以
// 指数形式加密，服务端在没有门限私钥的前提下无法还原「投了哪个选项」。
// 选票电路用同一套群运算，把「这份密文确实是本提案某个合法选项的加密」
// 变成可验证的零知识证明。
//
// 为什么不用仓库原先的 ed25519 密文栈：零知识电路的算术发生在 BN254 的
// 标量域里，电路内无法复算 ed25519 的群运算，也就无法证明「密文确实是
// 用本提案的选举密钥加密的」。把选票密文与电路统一到同一条曲线后，
// 密文关系才能进电路，能力 A（服务端拿不到明文选项）与能力 B（选票可
// 验证）才能同时成立。
package ballot

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	ed "github.com/consensys/gnark-crypto/ecc/bn254/twistededwards"
)

const (
	// ScalarSize 是 BN254 标量域元素的规范字节长度。
	ScalarSize = 32
	// PointSize 是压缩点编码的字节长度。
	PointSize = 32
	// MaxOptions 是单个提案的选项数上限，同时是电路里选项下标的位宽上限。
	MaxOptions = 256
)

// ErrInvalidPoint 表示点编码不合法、不在曲线上，或不落在素数阶子群里。
var ErrInvalidPoint = errors.New("invalid curve point")

// groupOrder 是 BN254 twisted Edwards 曲线素数阶子群的阶（标量域阶的 1/8）。
var groupOrder = ed.GetEdwardsCurve().Order

// Point 是 twisted Edwards 曲线上的仿射点。
type Point struct {
	X fr.Element
	Y fr.Element
}

// basePoint 返回曲线基点。
func basePoint() Point {
	params := ed.GetEdwardsCurve()
	return Point{X: params.Base.X, Y: params.Base.Y}
}

// identityPoint 返回单位元 (0, 1)。
func identityPoint() Point {
	var p Point
	p.X.SetZero()
	p.Y.SetOne()
	return p
}

// marshal 序列化为 32 字节压缩点。
func (p Point) marshal() []byte {
	affine := p.affine()
	return affine.Marshal()
}

// affine 转换成 gnark-crypto 的仿射点。
func (p Point) affine() ed.PointAffine {
	return ed.PointAffine{X: p.X, Y: p.Y}
}

// parsePoint 解析压缩点，并检查它是否在曲线上、是否落在素数阶子群里。
//
// 子群检查是必须的：这条曲线的余因子是 8，小阶点会让解密方泄漏私钥的
// 低位比特。凡是来自外部的点（选票密文里的两个点）都必须过这一关。
func parsePoint(raw []byte) (Point, error) {
	if len(raw) != PointSize {
		return Point{}, fmt.Errorf("%w: expected %d bytes, got %d", ErrInvalidPoint, PointSize, len(raw))
	}
	var affine ed.PointAffine
	if _, err := affine.SetBytes(raw); err != nil {
		return Point{}, fmt.Errorf("%w: %v", ErrInvalidPoint, err)
	}
	p := Point{X: affine.X, Y: affine.Y}
	if !p.isOnCurve() {
		return Point{}, fmt.Errorf("%w: point is not on the curve", ErrInvalidPoint)
	}
	if !p.isInSubGroup() {
		return Point{}, fmt.Errorf("%w: point is not in the prime order subgroup", ErrInvalidPoint)
	}
	return p, nil
}

// UncompressPoint 把压缩点转成未压缩形式：X 与 Y 各 32 字节大端，共 64 字节。
//
// 客户端用它做本地加密。浏览器端不必实现点解压（那需要在标量域里求平方根），
// 服务端直接给出坐标更省事，也让客户端能自己做曲线检查。
func UncompressPoint(publicKey []byte) ([]byte, error) {
	point, err := parsePoint(publicKey)
	if err != nil {
		return nil, err
	}
	affine := point.affine()
	x := affine.X.Bytes()
	y := affine.Y.Bytes()
	out := make([]byte, 0, 2*PointSize)
	out = append(out, x[:]...)
	out = append(out, y[:]...)
	return out, nil
}

// CompressPoint 把未压缩的曲线点（X 与 Y 各 32 字节大端）转成压缩形式。
//
// 它是 UncompressPoint 的逆操作。客户端拿到的计票公钥是未压缩形式（不必在
// 标量域里求平方根），但服务端拼公开输入时用的是压缩形式 —— 客户端要生成
// 能被服务端验过的证明，就得先转回压缩形式，两边才会算出同一份公开输入。
func CompressPoint(uncompressed []byte) ([]byte, error) {
	point, err := parseUncompressedPoint(uncompressed)
	if err != nil {
		return nil, err
	}
	return point.marshal(), nil
}

// parseUncompressedPoint 解析未压缩的曲线点（X 与 Y 各 32 字节大端）。
//
// 与压缩形式的区别：这里不需要在域内求平方根，客户端因此不必实现点解压。
// 子群检查同样要做 —— 小阶点会让解密方泄漏私钥低位比特。
func parseUncompressedPoint(raw []byte) (Point, error) {
	if len(raw) != 2*PointSize {
		return Point{}, fmt.Errorf(
			"%w: expected %d bytes, got %d", ErrInvalidPoint, 2*PointSize, len(raw),
		)
	}
	var x, y fr.Element
	x.SetBytes(raw[:PointSize])
	y.SetBytes(raw[PointSize:])
	p := Point{X: x, Y: y}
	if !p.isOnCurve() {
		return Point{}, fmt.Errorf("%w: point is not on the curve", ErrInvalidPoint)
	}
	if !p.isInSubGroup() {
		return Point{}, fmt.Errorf("%w: point is not in the prime order subgroup", ErrInvalidPoint)
	}
	return p, nil
}

// isOnCurve 检查点是否满足曲线方程。
func (p Point) isOnCurve() bool {
	affine := p.affine()
	return affine.IsOnCurve()
}

// isInSubGroup 用子群阶乘点；结果是单位元即说明点在素数阶子群里。
func (p Point) isInSubGroup() bool {
	var order fr.Element
	order.SetBigInt(&groupOrder)
	return p.scalarMul(order).isIdentity()
}

// isIdentity 判断是否是单位元。
func (p Point) isIdentity() bool {
	var one fr.Element
	one.SetOne()
	return p.X.IsZero() && p.Y.Equal(&one)
}

// equal 判断两个点是否是同一个群元素。
func (p Point) equal(q Point) bool {
	return p.X.Equal(&q.X) && p.Y.Equal(&q.Y)
}

// add 计算 p + q。
func (p Point) add(q Point) Point {
	left := p.affine()
	right := q.affine()
	sum := left.Add(&left, &right)
	return Point{X: sum.X, Y: sum.Y}
}

// sub 计算 p - q。
func (p Point) sub(q Point) Point {
	right := q.affine()
	right.Neg(&right)
	left := p.affine()
	diff := left.Add(&left, &right)
	return Point{X: diff.X, Y: diff.Y}
}

// scalarMul 计算 s·p。
//
// 群阶是标量域阶的 1/8，因此这里的标量乘法天然按子群阶归约：与电路
// 里的双倍加法结果一致（电路同样在子群里做群运算）。
func (p Point) scalarMul(s fr.Element) Point {
	var scalar big.Int
	s.ToBigIntRegular(&scalar)
	base := p.affine()
	out := base.ScalarMul(&base, &scalar)
	return Point{X: out.X, Y: out.Y}
}

// randomScalar 均匀采样一个标量。
func randomScalar() (fr.Element, error) {
	var buf [ScalarSize]byte
	for {
		if _, err := rand.Read(buf[:]); err != nil {
			return fr.Element{}, fmt.Errorf("read random bytes: %w", err)
		}
		// SetBytes 会把超范围输入按模 r 归约；这里直接拒绝而不是归约，
		// 保证采样分布是均匀的。
		if new(big.Int).SetBytes(buf[:]).Cmp(frModulus()) >= 0 {
			continue
		}
		var s fr.Element
		s.SetBytes(buf[:])
		return s, nil
	}
}

// frModulus 返回 BN254 标量域的模数。
func frModulus() *big.Int {
	return new(big.Int).Set(fr.Modulus())
}
