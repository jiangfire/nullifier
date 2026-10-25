package ballot

import (
	"fmt"

	"github.com/consensys/gnark-crypto/ecc/twistededwards"
	"github.com/consensys/gnark/frontend"
	edstd "github.com/consensys/gnark/std/algebra/twistededwards"
	"github.com/consensys/gnark/std/hash/mimc"
)

// optionIndexWidth 是选项下标在电路里的位宽：8 位，对应最多 256 个选项。
const optionIndexWidth = 8

// ballotCircuit 是选票的零知识电路。
//
// 公开输入（服务端自行拼装，客户端无从篡改）：
//
//	ProposalID    提案 ID 映射出的域元素
//	Nullifier     nullifier（由 nullifier 秘密与提案 ID 推导）
//	ElectionKeyXY 提案的计票公钥
//	CipherAXY     密文的第一个曲线点 a = r·G
//	CipherBXY     密文的第二个曲线点 b = m·G + r·Y
//	NumOptions    提案的选项数
//
// 私有见证：
//
//	OptionIndex     选项下标 m（服务端看不到，也拿不到）
//	Nonce           一次性随机数 r
//	NullifierSecret nullifier 秘密
//
// 电路断言三件事：
//
//  1. a = r·G，且 b = m·G + r·Y —— 密文确实是「用本提案的计票公钥、
//     某个合法选项下标」加密出来的；
//  2. m < NumOptions —— 选项下标落在本提案的选项范围内；
//  3. Nullifier = MiMC(NullifierSecret, ProposalID) —— nullifier 由秘密
//     推导且绑定本提案，既不能冒用别人的 nullifier，也不能跨提案复用。
//
// 路线图早期版本把选项下标列进公开输入，那会让服务端直接拿到明文选项，
// 与能力 A 冲突；这里把下标降为私有见证，用范围检查代替公开披露。
type ballotCircuit struct {
	// 公开输入。字段顺序即 witness 的填充顺序，改动会让已签发密钥失效。
	ProposalID   frontend.Variable `gnark:",public"`
	Nullifier    frontend.Variable `gnark:",public"`
	ElectionKeyX frontend.Variable `gnark:",public"`
	ElectionKeyY frontend.Variable `gnark:",public"`
	CipherAX     frontend.Variable `gnark:",public"`
	CipherAY     frontend.Variable `gnark:",public"`
	CipherBX     frontend.Variable `gnark:",public"`
	CipherBY     frontend.Variable `gnark:",public"`
	NumOptions   frontend.Variable `gnark:",public"`

	// 私有见证。
	OptionIndex     frontend.Variable
	Nonce           frontend.Variable
	NullifierSecret frontend.Variable
}

// Define 描述选票电路的约束。
func (c *ballotCircuit) Define(api frontend.API) error {
	curve, err := edstd.NewEdCurve(api, twistededwards.BN254)
	if err != nil {
		return fmt.Errorf("init twisted edwards curve: %w", err)
	}
	params := curve.Params()
	base := edstd.Point{
		X: params.Base[0],
		Y: params.Base[1],
	}
	electionKey := edstd.Point{X: c.ElectionKeyX, Y: c.ElectionKeyY}

	// a = r·G
	computedA := curve.ScalarMul(base, c.Nonce)
	api.AssertIsEqual(computedA.X, c.CipherAX)
	api.AssertIsEqual(computedA.Y, c.CipherAY)

	// b = m·G + r·Y
	computedB := curve.DoubleBaseScalarMul(base, electionKey, c.OptionIndex, c.Nonce)
	api.AssertIsEqual(computedB.X, c.CipherBX)
	api.AssertIsEqual(computedB.Y, c.CipherBY)

	// m 必须在提案的选项范围内
	if err := assertLessThan(api, c.OptionIndex, c.NumOptions, optionIndexWidth); err != nil {
		return err
	}

	// nullifier = MiMC(nullifier 秘密, 提案 ID)
	hash, err := mimc.NewMiMC(api)
	if err != nil {
		return fmt.Errorf("init mimc: %w", err)
	}
	hash.Write(c.NullifierSecret, c.ProposalID)
	api.AssertIsEqual(hash.Sum(), c.Nullifier)

	return nil
}

// assertLessThan 约束 value < limit。
//
// value 由调用方保证已经约束在 width 位以内，因此：
//
//	value < limit  ⟺  value + (2^width - limit) < 2^width
//
// 于是只要后者的 width+1 位分解里最高位为 0，就等价于 value < limit。
// limit 必须落在 (0, 2^width] 区间内，否则这条约束没有意义。
func assertLessThan(api frontend.API, value, limit frontend.Variable, width int) error {
	if width <= 0 || width > 16 {
		return fmt.Errorf("unsupported comparison width: %d", width)
	}
	// 把 value 约束成 width 位以内的数，并复原一次，保证位分解与 value 绑定。
	valueBits := api.ToBinary(value, width)
	var rebuilt frontend.Variable = apiConstant(0)
	for i := width - 1; i >= 0; i-- {
		rebuilt = api.Add(api.Mul(rebuilt, 2), valueBits[i])
	}
	api.AssertIsEqual(rebuilt, value)

	// 2^width - limit：limit 是公开输入，这里做一次域减法即可。
	bound := api.Sub(apiConstant(1<<width), limit)
	shifted := api.Add(value, bound)

	// shifted 的最高位为 0 ⟺ shifted < 2^width ⟺ value < limit。
	bits := api.ToBinary(shifted, width+1)
	api.AssertIsEqual(bits[width], 0)
	return nil
}

// apiConstant 返回一个常量变量。
func apiConstant(value int64) frontend.Variable {
	return frontend.Variable(value)
}
