package crypto

import (
	"errors"
	"fmt"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/crypto/ballot"
)

// GnarkProofVerifier 是选票零知识证明的验证器适配层。
type GnarkProofVerifier struct {
	verifier *ballot.Verifier
	// skipVerify 表示没有配置验证密钥。开发模式下不校验证明内容，只要求
	// 证明非空；release 模式强制要求 crypto.vk_path，这条分支不会生效。
	skipVerify bool
}

var _ domaincrypto.ProofVerifier = (*GnarkProofVerifier)(nil)

func NewGnarkProofVerifier(vkPath string) (*GnarkProofVerifier, error) {
	if vkPath == "" {
		// 开发环境允许不加载验证密钥，便于快速启动。
		return &GnarkProofVerifier{
			skipVerify: true,
		}, nil
	}

	verifier, err := ballot.NewVerifier(vkPath)
	if err != nil {
		return nil, err
	}
	return &GnarkProofVerifier{
		verifier: verifier,
	}, nil
}

// Verify 校验选票证明。
//
// 公开输入由调用方按请求字段、提案与计票会话拼装，因此一份 proof 只能
// 用于它被生成的那份密文：跨提案、换密文、改 nullifier 都会验证失败。
func (v *GnarkProofVerifier) Verify(proof *entity.Proof, statement entity.BallotStatement) error {
	if proof == nil || len(proof.Data) == 0 {
		return errors.New("proof is empty")
	}
	if v.skipVerify {
		return nil
	}

	inputs, err := ballot.BuildPublicInputs(statement)
	if err != nil {
		return fmt.Errorf("build public inputs: %w", err)
	}
	return v.verifier.Verify(proof.Data, inputs)
}
