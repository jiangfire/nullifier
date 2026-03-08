package crypto

import (
	"bytes"
	"errors"
	"fmt"
	"os"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"

	"github.com/consensys/gnark-crypto/ecc"
	"github.com/consensys/gnark/backend/groth16"
	"github.com/consensys/gnark/backend/witness"
)

// GnarkProofVerifier 是 gnark 的证明验证器适配层。
type GnarkProofVerifier struct {
	verifyingKey      groth16.VerifyingKey
	requireVerifyMode bool
}

var _ domaincrypto.ProofVerifier = (*GnarkProofVerifier)(nil)

func NewGnarkProofVerifier(vkPath string) (*GnarkProofVerifier, error) {
	if vkPath == "" {
		// 开发环境允许不加载验证密钥，便于快速启动。
		return &GnarkProofVerifier{
			requireVerifyMode: false,
		}, nil
	}

	raw, err := os.ReadFile(vkPath)
	if err != nil {
		return nil, fmt.Errorf("read verifying key: %w", err)
	}

	vk := groth16.NewVerifyingKey(ecc.BN254)
	if _, err := vk.ReadFrom(bytes.NewReader(raw)); err != nil {
		return nil, fmt.Errorf("decode verifying key: %w", err)
	}
	return &GnarkProofVerifier{
		verifyingKey:      vk,
		requireVerifyMode: true,
	}, nil
}

func (v *GnarkProofVerifier) Verify(proof *entity.Proof, inputs entity.PublicInputs) error {
	if proof == nil || len(proof.Data) == 0 {
		return errors.New("proof is empty")
	}
	if len(inputs) == 0 {
		return errors.New("public inputs are empty")
	}

	gnarkProof := groth16.NewProof(ecc.BN254)
	if _, err := gnarkProof.ReadFrom(bytes.NewReader(proof.Data)); err != nil {
		return fmt.Errorf("decode proof: %w", err)
	}

	if !v.requireVerifyMode {
		return nil
	}

	publicWitnessBytes, ok := inputs["public_witness"]
	if !ok || len(publicWitnessBytes) == 0 {
		return errors.New("public_witness is required")
	}

	publicWitness, err := witness.New(ecc.BN254, nil)
	if err != nil {
		return fmt.Errorf("init witness: %w", err)
	}
	if err := publicWitness.UnmarshalBinary(publicWitnessBytes); err != nil {
		return fmt.Errorf("decode public witness: %w", err)
	}

	if err := groth16.Verify(gnarkProof, v.verifyingKey, publicWitness); err != nil {
		return fmt.Errorf("verify proof: %w", err)
	}
	return nil
}
