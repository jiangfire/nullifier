package crypto

import (
	"fmt"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/infrastructure/crypto/ballot"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// BallotNullifierGenerator 用选票密码学栈生成 nullifier。
type BallotNullifierGenerator struct{}

var _ domaincrypto.NullifierGenerator = (*BallotNullifierGenerator)(nil)

// NewBallotNullifierGenerator 创建 nullifier 生成器。
func NewBallotNullifierGenerator() *BallotNullifierGenerator {
	return &BallotNullifierGenerator{}
}

// GenerateNullifier 生成一个规范编码的 nullifier。
//
// 采样在标量域内完成，因此返回值一定是规范编码：同一个值不会出现两种
// 字节表示，双重投票的唯一索引才拦得住。
func (g *BallotNullifierGenerator) GenerateNullifier() (string, error) {
	secret, err := ballot.GenerateNullifierSecret()
	if err != nil {
		return "", fmt.Errorf("generate nullifier secret: %w", err)
	}
	var scalar fr.Element
	scalar.SetBytes(secret)
	return ballot.EncodeScalarHex(scalar), nil
}
