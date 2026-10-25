package crypto

import (
	"context"
	"testing"

	domaincrypto "anonvote/internal/domain/crypto"

	"github.com/stretchr/testify/require"
)

// TestCredentialBlindClient_RoundTrip 覆盖盲签名客户端侧的完整往返：
// 本地盲化 → 签发方盲签 → 本地解盲 → 签发方验签。
//
// 这是 Go 客户端（CLI、端到端测试）与服务端之间那条「签发方只见盲化消息」的
// 链路的最小证明。
func TestCredentialBlindClient_RoundTrip(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	issuer := NewKryptologyCredentialIssuer()
	issuerKey, err := issuer.IssuerPublicKey(ctx)
	require.NoError(t, err)

	client, err := NewCredentialBlindClient(issuerKey)
	require.NoError(t, err)
	require.Equal(t, issuerKey.Version, client.KeyVersion())

	userPubKey := []byte("one-time-public-key")
	blinded, err := client.Blind("u-1", userPubKey)
	require.NoError(t, err)
	require.NotEmpty(t, blinded.BlindedMessage)

	// 签发方拿到的只是盲化消息 —— 它没有、也不需要消息原文。
	blindSignature, err := issuer.BlindSign(ctx, issuerKey.Version, blinded.BlindedMessage)
	require.NoError(t, err)

	signature, err := client.Finalize(blinded, blindSignature)
	require.NoError(t, err)

	version, err := issuer.VerifyCredential("u-1", userPubKey, signature)
	require.NoError(t, err)
	require.Equal(t, issuerKey.Version, version)
}

// TestCredentialBlindClient_CrossAttackFails 验证交叉攻击：拿 A 的盲签名去签 B
// 必然验签失败 —— 验签绑定的消息里含 user_id 与公钥。
func TestCredentialBlindClient_CrossAttackFails(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	issuer := NewKryptologyCredentialIssuer()
	issuerKey, err := issuer.IssuerPublicKey(ctx)
	require.NoError(t, err)

	client, err := NewCredentialBlindClient(issuerKey)
	require.NoError(t, err)

	blinded, err := client.Blind("u-alice", []byte("alice-key"))
	require.NoError(t, err)
	blindSignature, err := issuer.BlindSign(ctx, issuerKey.Version, blinded.BlindedMessage)
	require.NoError(t, err)
	signature, err := client.Finalize(blinded, blindSignature)
	require.NoError(t, err)

	// 换成 Bob 的身份去验 —— 必然过不去。
	_, err = issuer.VerifyCredential("u-bob", []byte("bob-key"), signature)
	require.Error(t, err)
}

// TestCredentialBlindClient_RejectsBadPublicKey 覆盖公钥校验。
func TestCredentialBlindClient_RejectsBadPublicKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		key  *domaincrypto.IssuerPublicKey
		want string
	}{
		{
			name: "nil key",
			key:  nil,
			want: "issuer public key is required",
		},
		{
			name: "missing modulus",
			key:  &domaincrypto.IssuerPublicKey{Exponent: 65537},
			want: "modulus is required",
		},
		{
			name: "missing exponent",
			key:  &domaincrypto.IssuerPublicKey{Modulus: []byte{1, 2, 3}},
			want: "exponent is required",
		},
		{
			name: "unsupported variant",
			key: &domaincrypto.IssuerPublicKey{
				Modulus:  []byte{1, 2, 3},
				Exponent: 65537,
				Variant:  "SHA256PSS",
			},
			want: "unsupported blind signature variant",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewCredentialBlindClient(testCase.key)
			require.Error(t, err)
			require.Contains(t, err.Error(), testCase.want)
		})
	}
}

// TestCredentialBlindClient_RejectsBadInput 覆盖盲化与解盲的入参校验。
func TestCredentialBlindClient_RejectsBadInput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	issuer := NewKryptologyCredentialIssuer()
	issuerKey, err := issuer.IssuerPublicKey(ctx)
	require.NoError(t, err)
	client, err := NewCredentialBlindClient(issuerKey)
	require.NoError(t, err)

	_, err = client.Blind("", []byte("key"))
	require.ErrorContains(t, err, "user_id is required")

	_, err = client.Blind("u-1", nil)
	require.ErrorContains(t, err, "user_pub_key is required")

	_, err = client.Finalize(nil, []byte("sig"))
	require.ErrorContains(t, err, "blinded credential is required")

	blinded, err := client.Blind("u-1", []byte("key"))
	require.NoError(t, err)

	_, err = client.Finalize(blinded, nil)
	require.ErrorContains(t, err, "blind signature is required")
}
