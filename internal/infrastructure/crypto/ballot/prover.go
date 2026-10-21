package ballot

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

// ErrInvalidProof 表示证明无法解析或未通过验证。
var ErrInvalidProof = errors.New("invalid ballot proof")

// Prover 用 proving key 生成选票证明。
//
// proving key 只在客户端侧使用（浏览器 WASM 或 CLI），不进服务端。
type Prover struct {
	compiled frontend.CompiledConstraintSystem
	key      groth16.ProvingKey
}

// NewProver 从 proving key 文件加载证明器。
func NewProver(provingKeyPath string) (*Prover, error) {
	raw, err := os.ReadFile(provingKeyPath)
	if err != nil {
		return nil, fmt.Errorf("read proving key: %w", err)
	}
	key := groth16.NewProvingKey(ecc.BN254)
	if _, err := key.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("decode proving key: %w", err)
	}
	compiled, err := compileCircuit()
	if err != nil {
		return nil, err
	}
	return &Prover{compiled: compiled, key: key}, nil
}

// Prove 生成选票证明。
//
// inputs 必须是与密文、提案、nullifier 一致的公开输入；optionIndex 是
// 选中的选项下标，nonce 是加密用的一次性随机数，nullifierSecret 是
// 投票人的 nullifier 秘密。
func (p *Prover) Prove(
	inputs PublicInputs,
	optionIndex uint64,
	nonce fr.Element,
	nullifierSecret fr.Element,
) ([]byte, error) {
	if optionIndex >= MaxOptions {
		return nil, fmt.Errorf("%w: option index out of range: %d", ErrInvalidProof, optionIndex)
	}
	var option fr.Element
	option.SetUint64(optionIndex)

	assignment := &ballotCircuit{
		ProposalID:      toBigInt(inputs.ProposalID),
		Nullifier:       toBigInt(inputs.Nullifier),
		ElectionKeyX:    toBigInt(inputs.ElectionKey.X),
		ElectionKeyY:    toBigInt(inputs.ElectionKey.Y),
		CipherAX:        toBigInt(inputs.CipherA.X),
		CipherAY:        toBigInt(inputs.CipherA.Y),
		CipherBX:        toBigInt(inputs.CipherB.X),
		CipherBY:        toBigInt(inputs.CipherB.Y),
		NumOptions:      toBigInt(inputs.NumOptions),
		OptionIndex:     toBigInt(option),
		Nonce:           toBigInt(nonce),
		NullifierSecret: toBigInt(nullifierSecret),
	}
	full, err := frontend.NewWitness(assignment, ecc.BN254)
	if err != nil {
		return nil, fmt.Errorf("build witness: %w", err)
	}
	proof, err := groth16.Prove(p.compiled, p.key, full)
	if err != nil {
		return nil, fmt.Errorf("prove ballot: %w", err)
	}
	var buf bytes.Buffer
	if _, err := proof.WriteTo(&buf); err != nil {
		return nil, fmt.Errorf("serialize proof: %w", err)
	}
	return buf.Bytes(), nil
}

// Verifier 用 verifying key 校验选票证明。
type Verifier struct {
	key groth16.VerifyingKey
}

// NewVerifier 从 verifying key 文件加载验证器。
func NewVerifier(vkPath string) (*Verifier, error) {
	raw, err := os.ReadFile(vkPath)
	if err != nil {
		return nil, fmt.Errorf("read verifying key: %w", err)
	}
	key := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := key.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("decode verifying key: %w", err)
	}
	return &Verifier{key: key}, nil
}

// Verify 校验选票证明是否与给定的公开输入匹配。
func (v *Verifier) Verify(proofBytes []byte, inputs PublicInputs) error {
	if len(proofBytes) == 0 {
		return fmt.Errorf("%w: proof is empty", ErrInvalidProof)
	}
	proof := groth16.NewProof(ecc.BN254)
	if _, err := proof.ReadFrom(bytes.NewReader(proofBytes)); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidProof, err)
	}
	assignment := &ballotCircuit{
		ProposalID:   toBigInt(inputs.ProposalID),
		Nullifier:    toBigInt(inputs.Nullifier),
		ElectionKeyX: toBigInt(inputs.ElectionKey.X),
		ElectionKeyY: toBigInt(inputs.ElectionKey.Y),
		CipherAX:     toBigInt(inputs.CipherA.X),
		CipherAY:     toBigInt(inputs.CipherA.Y),
		CipherBX:     toBigInt(inputs.CipherB.X),
		CipherBY:     toBigInt(inputs.CipherB.Y),
		NumOptions:   toBigInt(inputs.NumOptions),
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

// GenerateKeys 生成选票电路的 proving key 与 verifying key。
//
// verifying key 部署到服务端；proving key 只在客户端侧使用，不能入库。
func GenerateKeys() (provingKey []byte, verifyingKey []byte, err error) {
	compiled, err := compileCircuit()
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

// compileCircuit 编译选票电路。
func compileCircuit() (frontend.CompiledConstraintSystem, error) {
	compiled, err := frontend.Compile(ecc.BN254, r1cs.NewBuilder, &ballotCircuit{})
	if err != nil {
		return nil, fmt.Errorf("compile ballot circuit: %w", err)
	}
	return compiled, nil
}

// toBigInt 把域元素转成常规形式的 big.Int。
func toBigInt(element fr.Element) *big.Int {
	var value big.Int
	element.ToBigIntRegular(&value)
	return &value
}
