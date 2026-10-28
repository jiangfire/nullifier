package eligibility

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"os"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/frontend"
	"github.com/consensys/gnark/frontend/cs/r1cs"
)

// ErrInvalidProof 表示资格证明无法解析或未通过验证。
var ErrInvalidProof = errors.New("invalid eligibility proof")

// Prover 用 proving key 生成资格证明。
//
// proving key 只在客户端侧使用（浏览器 WASM 或 CLI），不进服务端。
// depth 是本实例编译电路时的树深 —— proving key 是 per-depth 的，
// 深度不匹配的成员包在这里就会被拒。
type Prover struct {
	compiled frontend.CompiledConstraintSystem
	key      groth16.ProvingKey
	depth    int
}

// NewProver 从 proving key 文件加载证明器（默认树深）。
func NewProver(provingKeyPath string) (*Prover, error) {
	return NewProverAtDepth(provingKeyPath, DefaultTreeDepth)
}

// NewProverAtDepth 从 proving key 文件加载证明器，按给定树深编译电路。
//
// proving key 是按深度生成的：密钥与深度的配对错了，出证明这一步就会失败 ——
// 宁可在这里失败，也不要出一份注定验不过的证明。
func NewProverAtDepth(provingKeyPath string, depth int) (*Prover, error) {
	raw, err := os.ReadFile(provingKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read proving key: %w", err)
	}
	return NewProverFromBytesAtDepth(raw, depth)
}

// NewProverFromBytes 从 proving key 字节流加载证明器（默认树深）。
//
// 浏览器端没有文件路径：proving key 由门户下载后直接交给 WASM 模块，
// 走的就是这个入口。
func NewProverFromBytes(raw []byte) (*Prover, error) {
	return NewProverFromBytesAtDepth(raw, DefaultTreeDepth)
}

// NewProverFromBytesAtDepth 从 proving key 字节流加载证明器，按给定树深编译电路。
func NewProverFromBytesAtDepth(raw []byte, depth int) (*Prover, error) {
	if err := ValidateTreeDepth(depth); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("decode proving key: empty proving key")
	}
	key := groth16.NewProvingKey(ecc.BN254)
	if _, err := key.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("decode proving key: %w", err)
	}
	compiled, err := compileCircuit(depth)
	if err != nil {
		return nil, err
	}
	return &Prover{compiled: compiled, key: key, depth: depth}, nil
}

// Depth 返回本证明器编译电路时的树深。
func (p *Prover) Depth() int {
	return p.depth
}

// Prove 生成资格证明。
//
// statement 是公开输入；secret 是资格秘密；index 是叶子位置；path 是 Merkle 路径。
func (p *Prover) Prove(
	statement Statement,
	secret fr.Element,
	index uint64,
	path []fr.Element,
) ([]byte, error) {
	if len(path) != p.depth {
		return nil, fmt.Errorf("%w: merkle path must have %d elements, got %d",
			ErrInvalidProof, p.depth, len(path))
	}
	pathVars := make([]frontend.Variable, len(path))
	for i := range path {
		pathVars[i] = toBigInt(path[i])
	}

	assignment := &eligibilityCircuit{
		Root:            toBigInt(statement.Root),
		ProposalID:      toBigInt(statement.ProposalID),
		Challenge:       toBigInt(statement.Challenge),
		QualificationID: toBigInt(statement.QualificationID),
		SessionBinding:  toBigInt(statement.SessionBinding),
		Secret:          toBigInt(secret),
		Index:           new(big.Int).SetUint64(index),
		Path:            pathVars,
	}
	full, err := frontend.NewWitness(assignment, ecc.BN254)
	if err != nil {
		return nil, fmt.Errorf("build witness: %w", err)
	}
	proof, err := groth16.Prove(p.compiled, p.key, full)
	if err != nil {
		return nil, fmt.Errorf("prove eligibility: %w", err)
	}
	var buf bytes.Buffer
	if _, err := proof.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("serialize proof: %w", err)
	}
	return buf.Bytes(), nil
}

// Verifier 用 verifying key 校验资格证明。
type Verifier struct {
	key groth16.VerifyingKey
}

// NewVerifier 从 verifying key 文件加载验证器。
func NewVerifier(vkPath string) (*Verifier, error) {
	raw, err := os.ReadFile(vkPath)
	if err != nil {
		return nil, fmt.Errorf("read verifying key: %w", err)
	}
	return NewVerifierFromBytes(raw)
}

// NewVerifierFromBytes 从 verifying key 字节流加载验证器。
func NewVerifierFromBytes(raw []byte) (*Verifier, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("decode verifying key: empty verifying key")
	}
	key := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := key.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("decode verifying key: %w", err)
	}
	return &Verifier{key: key}, nil
}

// Verify 校验资格证明是否与给定的公开输入匹配。
func (v *Verifier) Verify(proofBytes []byte, statement Statement) error {
	if len(proofBytes) == 0 {
		return fmt.Errorf("%w: proof is empty", ErrInvalidProof)
	}
	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProof, err)
	}
	assignment := &eligibilityCircuit{
		Root:            toBigInt(statement.Root),
		ProposalID:      toBigInt(statement.ProposalID),
		Challenge:       toBigInt(statement.Challenge),
		QualificationID: toBigInt(statement.QualificationID),
		SessionBinding:  toBigInt(statement.SessionBinding),
	}
	public, err := frontend.NewWitness(assignment, ecc.BN254, frontend.PublicOnly())
	if err != nil {
		return fmt.Errorf("%w: build public witness: %v", ErrInvalidProof, err)
	}
	if err := groth16.Verify(proof, v.key, public); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProof, err)
	}
	return nil
}

// GenerateKeys 生成资格电路的 proving key 与 verifying key（按给定树深）。
//
// verifying key 部署到服务端；proving key 只在客户端侧使用，不能入库。
// **每深度一套密钥**：换深度 = 重新生成密钥 = 重新生成名单，三项必须成套。
func GenerateKeys(depth int) (provingKey []byte, verifyingKey []byte, err error) {
	compiled, err := compileCircuit(depth)
	if err != nil {
		return nil, nil, err
	}
	pk, vk, err := groth16.Setup(compiled)
	if err != nil {
		return nil, nil, fmt.Errorf("setup groth16: %w", err)
	}
	var pkBuf, vkBuf bytes.Buffer
	if _, err := pk.WriteTo(&pkBuf); err != nil {
		return nil, nil, fmt.Errorf("serialize proving key: %w", err)
	}
	if _, err := vk.WriteTo(&vkBuf); err != nil {
		return nil, nil, fmt.Errorf("serialize verifying key: %w", err)
	}
	return pkBuf.Bytes(), vkBuf.Bytes(), nil
}

// Constraints 返回电路在给定树深下的约束条数（供文档与测试引用）。
func Constraints(depth int) (int, error) {
	compiled, err := compileCircuit(depth)
	if err != nil {
		return 0, err
	}
	return compiled.GetNbConstraints(), nil
}

// compileCircuit 按给定树深编译资格电路。
//
// 电路形状由模板实例的 Path 切片长度决定 —— 深度因此是运行期参数，
// 而不是编译期常量。宽度（位分解、上溯层数）在 Define 里取 len(c.Path)。
func compileCircuit(depth int) (frontend.CompiledConstraintSystem, error) {
	if err := ValidateTreeDepth(depth); err != nil {
		return nil, err
	}
	compiled, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &eligibilityCircuit{
		Path: make([]frontend.Variable, depth),
	})
	if err != nil {
		return nil, fmt.Errorf("compile eligibility circuit: %w", err)
	}
	return compiled, nil
}

// toBigInt 把域元素转成常规形式的 big.Int。
func toBigInt(element fr.Element) *big.Int {
	var value big.Int
	element.ToBigIntRegular(&value)
	return &value
}
