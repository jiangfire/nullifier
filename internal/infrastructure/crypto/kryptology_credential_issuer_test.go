package crypto

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestKryptologyCredentialIssuer_IssueAndVerify(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	credential, err := issuer.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.NotEmpty(t, credential.Signature)

	err = issuer.VerifyCredential("user-1", []byte("user-public-key"), credential.Signature)
	require.NoError(t, err)
}

func TestKryptologyCredentialIssuer_VerifyRejectsTamperedSignature(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	credential, err := issuer.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotNil(t, credential)

	credential.Signature[len(credential.Signature)-1] ^= 0x01
	err = issuer.VerifyCredential("user-1", []byte("user-public-key"), credential.Signature)
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_VerifyRejectsDifferentIdentity(t *testing.T) {
	issuer := NewKryptologyCredentialIssuer()

	credential, err := issuer.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotNil(t, credential)

	err = issuer.VerifyCredential("user-2", []byte("user-public-key"), credential.Signature)
	require.Error(t, err)
}

func TestKryptologyCredentialIssuer_PersistKeyAcrossRestart(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "issuer_keys.json")

	issuer1 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	credential, err := issuer1.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.NotNil(t, credential)
	require.Equal(t, 1, credential.IssuerKeyVersion)

	issuer2 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	err = issuer2.VerifyCredential("user-1", []byte("user-public-key"), credential.Signature)
	require.NoError(t, err)
}

func TestKryptologyCredentialIssuer_RotateAndVerifyOldCredential(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "issuer_keys.json")

	issuer1 := NewKryptologyCredentialIssuerWithFile(storePath, 24*time.Hour)
	credential1, err := issuer1.Issue(context.Background(), "user-1", []byte("user-public-key"))
	require.NoError(t, err)
	require.Equal(t, 1, credential1.IssuerKeyVersion)

	issuer2 := NewKryptologyCredentialIssuerWithFile(storePath, time.Nanosecond)
	credential2, err := issuer2.Issue(context.Background(), "user-2", []byte("user-public-key-2"))
	require.NoError(t, err)
	require.Greater(t, credential2.IssuerKeyVersion, credential1.IssuerKeyVersion)

	err = issuer2.VerifyCredential("user-1", []byte("user-public-key"), credential1.Signature)
	require.NoError(t, err)
	err = issuer2.VerifyCredential("user-2", []byte("user-public-key-2"), credential2.Signature)
	require.NoError(t, err)
}
