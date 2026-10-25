package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// EligibilityService 是资格证明的服务端侧实现。
//
// 它只持有两样东西：**名单的根**与**资格电路的验证密钥**。没有名单、没有
// 成员包 —— 因此它能验证「这个资格在树里」，却无法把资格对应到任何成员。
//
// 与选票证明不同，这里**没有**「跳过验证」的降级模式：验签密钥缺失就等于
// 服务端无法履行一人一票的承诺，那必须在启动时失败，而不是悄悄放行。
type EligibilityService struct {
	root     string
	depth    int
	verifier *eligibility.Verifier
}

var _ domaincrypto.EligibilityService = (*EligibilityService)(nil)

// NewEligibilityService 造一个资格校验服务。
//
// 两个参数要么都给、要么都不给：只给根没有验证密钥等于「不校验」，只给验证
// 密钥没有根则无从验证。都不给表示**未开启**资格校验（开发环境的降级形态）。
func NewEligibilityService(root string, verifyingKeyPath string) (*EligibilityService, error) {
	root = strings.TrimSpace(root)
	verifyingKeyPath = strings.TrimSpace(verifyingKeyPath)

	switch {
	case root == "" && verifyingKeyPath == "":
		return &EligibilityService{}, nil
	case root == "":
		return nil, errors.New("crypto.eligibility_root is required when crypto.eligibility_vk_path is set")
	case verifyingKeyPath == "":
		return nil, errors.New("crypto.eligibility_vk_path is required when crypto.eligibility_root is set")
	}

	if _, err := eligibility.DecodeField(root); err != nil {
		return nil, fmt.Errorf("invalid crypto.eligibility_root: %w", err)
	}
	verifier, err := eligibility.NewVerifier(verifyingKeyPath)
	if err != nil {
		return nil, err
	}
	return &EligibilityService{
		root:     root,
		depth:    eligibility.TreeDepth,
		verifier: verifier,
	}, nil
}

// Enabled 报告资格校验是否开启。
func (s *EligibilityService) Enabled() bool {
	return s != nil && s.verifier != nil
}

// Root 返回名单树根与树深。
func (s *EligibilityService) Root() (string, int, error) {
	if !s.Enabled() {
		return "", 0, errors.New("eligibility verification is not configured")
	}
	return s.root, s.depth, nil
}

// NewChallenge 生成一个一次性的领取挑战。
//
// 挑战是 32 字节随机值**归约进标量域**后的规范编码：随机字节有约 2^-32 的
// 概率超过模数，直接当域元素用会出现「同一个值两种字节表示」。
func (s *EligibilityService) NewChallenge() (string, error) {
	if !s.Enabled() {
		return "", errors.New("eligibility verification is not configured")
	}
	raw := make([]byte, eligibility.FieldSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random challenge: %w", err)
	}
	var element fr.Element
	element.SetBytes(raw)
	return eligibility.EncodeField(element), nil
}

// Verify 校验一份资格证明。
//
// root 用**服务端自己配置的**那个，不接受调用方塞进来的：资格证明的意义
// 就在于「服务端认的是自己那份名单」。
func (s *EligibilityService) Verify(proof []byte, statement domaincrypto.EligibilityStatement) error {
	if !s.Enabled() {
		return errors.New("eligibility verification is not configured")
	}
	built, err := eligibility.BuildStatement(
		s.root,
		statement.ProposalID,
		statement.Challenge,
		statement.QualificationID,
	)
	if err != nil {
		return fmt.Errorf("build eligibility statement: %w", err)
	}
	return s.verifier.Verify(proof, built)
}
