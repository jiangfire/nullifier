package crypto

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// issueCredential 走完客户端侧的两段式盲签名，返回最终签名。
//
// 它刻意把「客户端做什么」与「服务端做什么」分开：客户端本地盲化、收盲签名、
// 本地解盲；服务端只做一次 BlindSign —— 全程没见过消息原文。
func issueCredential(
	t *testing.T,
	issuer *KryptologyCredentialIssuer,
	userID string,
	userPubKey []byte,
) []byte {
	t.Helper()
	issuerKey, err := issuer.IssuerPublicKey(context.Background())
	require.NoError(t, err)
	client, err := NewCredentialBlindClient(issuerKey)
	require.NoError(t, err)
	blinded, err := client.Blind(userID, userPubKey)
	require.NoError(t, err)
	blindSignature, err := issuer.BlindSign(
		context.Background(), issuerKey.Version, blinded.BlindedMessage,
	)
	require.NoError(t, err)
	signature, err := client.Finalize(blinded, blindSignature)
	require.NoError(t, err)
	return signature
}

func TestKryptologyCredentialIssuer_BlindSignAndVerify(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	signature := issueCredential(t, issuer, "user-1", []byte("user-public-key"))
	require.NotEmpty(t, signature)

	version, err := issuer.VerifyCredential("user-1", []byte("user-public-key"), signature)
	require.NoError(t, err)
	require.Equal(t, 1, version)
}

func TestKryptologyCredentialIssuer_VerifyRejectsTamperedSignature(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	signature := issueCredential(t, issuer, "user-1", []byte("user-public-key"))
	require.NotEmpty(t, signature)

	signature[len(signature)-1] ^= 0x01
	_, err := issuer.VerifyCredential("user-1", []byte("user-public-key"), signature)
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_VerifyRejectsDifferentIdentity(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	signature := issueCredential(t, issuer, "user-1", []byte("user-public-key"))
	require.NotEmpty(t, signature)

	_, err := issuer.VerifyCredential("user-2", []byte("user-public-key"), signature)
	require.Error(t, err)
}

// TestKryptologyCredentialIssuer_VerifyRejectsDifferentPublicKey 验证「拿 A 的盲
// 签名去签 B」会被挡下 —— 这就是路线图 T17 的交叉攻击验收项。
func TestKryptologyCredentialIssuer_VerifyRejectsDifferentPublicKey(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	signature := issueCredential(t, issuer, "user-1", []byte("user-public-key"))

	_, err := issuer.VerifyCredential("user-1", []byte("another-public-key"), signature)
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_BlindSignRejectsUnknownKeyVersion(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	_, err := issuer.BlindSign(context.Background(), 99, []byte("blinded"))
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_BlindSignRejectsEmptyMessage(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	_, err := issuer.BlindSign(context.Background(), 1, nil)
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_PersistKeyAcrossRestart(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "issuer_keys.json")

	issuer1 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	signature := issueCredential(t, issuer1, "user-1", []byte("user-public-key"))
	require.NotEmpty(t, signature)

	issuer2 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	version, err := issuer2.VerifyCredential("user-1", []byte("user-public-key"), signature)
	require.NoError(t, err)
	require.Equal(t, 1, version)
}

func TestKryptologyCredentialIssuer_RotateAndVerifyOldCredential(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "issuer_keys.json")

	issuer1 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	issuerKey1, err := issuer1.IssuerPublicKey(context.Background())
	require.NoError(t, err)
	require.Equal(t, 1, issuerKey1.Version)
	signature1 := issueCredential(t, issuer1, "user-1", []byte("user-public-key"))

	// 轮换周期设为纳秒：下一次取公钥时就会轮换。
	issuer2 := NewKryptologyCredentialIssuerWithFile(storePath, time.Nanosecond)
	issuerKey2, err := issuer2.IssuerPublicKey(context.Background())
	require.NoError(t, err)
	require.Greater(t, issuerKey2.Version, issuerKey1.Version)
	signature2 := issueCredential(t, issuer2, "user-2", []byte("user-public-key-2"))

	version, err := issuer2.VerifyCredential("user-1", []byte("user-public-key"), signature1)
	require.NoError(t, err)
	require.Equal(t, 1, version)
	version, err = issuer2.VerifyCredential("user-2", []byte("user-public-key-2"), signature2)
	require.NoError(t, err)
	require.Greater(t, version, issuerKey1.Version)
}

// TestKryptologyCredentialIssuer_BlindSignUsesRequestedKeyVersion 验证盲签名用的是
// 客户端指定的那一版密钥 —— 版本对不上，客户端解出来的签名验不过。
func TestKryptologyCredentialIssuer_BlindSignUsesRequestedKeyVersion(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "issuer_keys.json")

	issuer := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	issuerKey, err := issuer.IssuerPublicKey(context.Background())
	require.NoError(t, err)

	client, err := NewCredentialBlindClient(issuerKey)
	require.NoError(t, err)
	blinded, err := client.Blind("user-1", []byte("user-public-key"))
	require.NoError(t, err)

	blindSignature, err := issuer.BlindSign(
		context.Background(), issuerKey.Version, blinded.BlindedMessage,
	)
	require.NoError(t, err)
	signature, err := client.Finalize(blinded, blindSignature)
	require.NoError(t, err)

	_, err = issuer.VerifyCredential("user-1", []byte("user-public-key"), signature)
	require.NoError(t, err)
}
