package crypto

import (
	"fmt"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"
)

// BallotThresholdCrypto 用选票密码学栈实现计票会话的密钥管理与选票加解密。
//
// 选票密文与零知识电路跑在同一条曲线（BN254 twisted Edwards）上：电路
// 才能验证「这份密文确实是本提案的计票公钥加密的某个合法选项」。
type BallotThresholdCrypto struct{}

var _ domaincrypto.Encrypter = (*BallotThresholdCrypto)(nil)

// NewBallotThresholdCrypto 创建门限选票密码学实现。
func NewBallotThresholdCrypto() *BallotThresholdCrypto {
	return &BallotThresholdCrypto{}
}

// GenerateKeyPair 生成计票密钥对。
func (c *BallotThresholdCrypto) GenerateKeyPair() ([]byte, []byte, error) {
	keys, err := ballot.GenerateKeyPair()
	if err != nil {
		return nil, nil, err
	}
	return keys.Private, keys.Public, nil
}

// SplitPrivateKey 把计票私钥拆成门限份额。
func (c *BallotThresholdCrypto) SplitPrivateKey(privateKey []byte, threshold, trustees int) ([][]byte, error) {
	return ballot.SplitSecret(privateKey, threshold, trustees)
}

// RecoverPrivateKey 用门限份额还原计票私钥。
func (c *BallotThresholdCrypto) RecoverPrivateKey(shares [][]byte, threshold, trustees int) ([]byte, error) {
	return ballot.RecoverSecret(shares, threshold)
}

// Encrypt 用计票公钥加密选项下标。
func (c *BallotThresholdCrypto) Encrypt(publicKey []byte, optionIndex int) ([]byte, error) {
	if optionIndex < 0 {
		return nil, fmt.Errorf("option index must not be negative: %d", optionIndex)
	}
	ciphertext, _, err := ballot.EncryptBallot(publicKey, uint64(optionIndex))
	if err != nil {
		return nil, err
	}
	return ciphertext, nil
}

// Decrypt 用计票私钥还原选项下标。
func (c *BallotThresholdCrypto) Decrypt(privateKey []byte, ciphertext []byte, numOptions int) (int, error) {
	index, err := ballot.DecryptBallotOption(privateKey, ciphertext, numOptions)
	if err != nil {
		return 0, err
	}
	return int(index), nil
}

// ValidateCiphertext 校验选票密文的结构。
func (c *BallotThresholdCrypto) ValidateCiphertext(cipher []byte) error {
	return ballot.ValidateBallotCiphertext(cipher)
}
