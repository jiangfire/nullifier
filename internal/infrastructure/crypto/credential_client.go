package crypto

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"math/big"

	domaincrypto "anonvote/internal/domain/crypto"

	"github.com/cloudflare/circl/blindsign/blindrsa"
)

// CredentialBlindClient 是凭证盲签名的**客户端侧**。
//
// 关键事实（读 CIRCL 源码确认）：`Prepare` / `Blind` / `Finalize` 全是客户端
// 操作，只需要签发方的 **RSA 公钥**，不需要私钥、也不需要服务端配合。
// 服务端那一侧只有 `Signer.BlindSign(blindedMessage)` —— 也就是「签发方全程
// 只见盲化消息」。
//
// 这个类型让 Go 客户端（CLI、端到端测试）能当真正的盲签名客户端，与浏览器端
// （`web/src/lib/blind-rsa.ts`）走的是同一套协议。
type CredentialBlindClient struct {
	client    blindrsa.Client
	publicKey *rsa.PublicKey
	version   int
}

// BlindedCredential 是盲化后的消息，外加解盲要用的状态。
//
// 状态**不能**交给签发方 —— 它是解盲的钥匙。
type BlindedCredential struct {
	// BlindedMessage 是交给签发方去盲签的消息。
	BlindedMessage []byte
	// state 是 CIRCL 的解盲状态。
	state blindrsa.State
	// valid 区分「真的盲化过」与「零值」。
	valid bool
}

// NewCredentialBlindClient 用签发方公钥构造一个盲签名客户端。
func NewCredentialBlindClient(publicKey *domaincrypto.IssuerPublicKey) (*CredentialBlindClient, error) {
	if publicKey == nil {
		return nil, errors.New("issuer public key is required")
	}
	if len(publicKey.Modulus) == 0 {
		return nil, errors.New("issuer public key modulus is required")
	}
	if publicKey.Exponent <= 0 {
		return nil, errors.New("issuer public key exponent is required")
	}
	// 变体必须与服务端一致：变体不同，盲化结果就不同，签名验不过。
	if publicKey.Variant != blindrsa.SHA384PSSDeterministic.String() {
		return nil, fmt.Errorf(
			"unsupported blind signature variant %q", publicKey.Variant,
		)
	}

	modulus := new(big.Int).SetBytes(publicKey.Modulus)
	key := &rsa.PublicKey{N: modulus, E: publicKey.Exponent}
	client, err := blindrsa.NewClient(blindrsa.SHA384PSSDeterministic, key)
	if err != nil {
		return nil, fmt.Errorf("new blind rsa client: %w", err)
	}
	return &CredentialBlindClient{
		client:    client,
		publicKey: key,
		version:   publicKey.Version,
	}, nil
}

// KeyVersion 返回这个客户端盲化时用的密钥版本。
//
// 客户端要把它一起发给签发方：签发方必须用同一把私钥盲签。
func (c *CredentialBlindClient) KeyVersion() int {
	return c.version
}

// Blind 把凭证消息盲化，交给签发方去盲签。
//
// message 是 `sha256(user_id ‖ 0x00 ‖ user_pub_key)` —— 服务端验签时用的是
// 同一条消息。
func (c *CredentialBlindClient) Blind(userID string, userPubKey []byte) (*BlindedCredential, error) {
	if userID == "" {
		return nil, errors.New("user_id is required")
	}
	if len(userPubKey) == 0 {
		return nil, errors.New("user_pub_key is required")
	}

	message := CredentialMessage(userID, userPubKey)
	prepared, err := c.client.Prepare(rand.Reader, message)
	if err != nil {
		return nil, fmt.Errorf("prepare blind message: %w", err)
	}
	blinded, state, err := c.client.Blind(rand.Reader, prepared)
	if err != nil {
		return nil, fmt.Errorf("blind message: %w", err)
	}
	return &BlindedCredential{BlindedMessage: blinded, state: state, valid: true}, nil
}

// Finalize 解盲：把签发方返回的盲签名还原成最终签名。
func (c *CredentialBlindClient) Finalize(
	blinded *BlindedCredential,
	blindSignature []byte,
) ([]byte, error) {
	if blinded == nil || !blinded.valid {
		return nil, errors.New("blinded credential is required")
	}
	if len(blindSignature) == 0 {
		return nil, errors.New("blind signature is required")
	}
	signature, err := c.client.Finalize(blinded.state, blindSignature)
	if err != nil {
		return nil, fmt.Errorf("finalize blind signature: %w", err)
	}
	return signature, nil
}
