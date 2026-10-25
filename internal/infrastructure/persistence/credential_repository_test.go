package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"

	"github.com/stretchr/testify/require"
)

// TestCredentialClaimRepository_GetAvailableByUserID 覆盖按 user_id 查空槽位。
//
// 真两段式盲签名用它把盲签名绑定到具体槽位：公开端点只能为池里还空着的槽位
// 盲签 —— 否则签发方就成了「任意消息」的签名预言机。
func TestCredentialClaimRepository_GetAvailableByUserID(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 2)

	claim, err := repo.GetAvailableByUserID(ctx, "p-1", "pseudonym-a")
	require.NoError(t, err)
	require.Equal(t, "pseudonym-a", claim.UserID)
	require.Equal(t, "slot-p-1-a", claim.ID)
}

// 槽位被领走之后就不能再按 user_id 查到了 —— 它已经不在「可用」集合里。
func TestCredentialClaimRepository_GetAvailableByUserIDSkipsClaimed(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 1)

	claim, err := repo.TakeAvailable(ctx, "p-1")
	require.NoError(t, err)
	ok, err := repo.MarkClaimed(ctx, claim.ID, []byte("pub"), []byte("sig"), time.Now().UTC())
	require.NoError(t, err)
	require.True(t, ok)

	_, err = repo.GetAvailableByUserID(ctx, "p-1", claim.UserID)
	require.ErrorIs(t, err, repository.ErrNotFound)
}

// 别的提案里的同名单数槽位不能被拿来签 —— 槽位绑定是 (proposal, user_id)。
func TestCredentialClaimRepository_GetAvailableByUserIDIsScopedByProposal(t *testing.T) {
	repo := newClaimRepo(t)
	ctx := context.Background()
	seedClaim(t, repo, "p-1", 1)

	_, err := repo.GetAvailableByUserID(ctx, "p-2", "pseudonym-a")
	require.ErrorIs(t, err, repository.ErrNotFound)
}

func newCredentialRepo(t *testing.T) *CredentialRepository {
	t.Helper()
	db, err := NewDB(&DBConfig{Driver: "sqlite", Database: ":memory:"})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return NewCredentialRepository(db)
}

// TestCredentialRepository_SaveRejectsDuplicate 验证同一提案下的 user_id 唯一。
//
// 并发 attest 时靠它兜底：两个客户端抢到同一个槽位时，只能有一个落库。
func TestCredentialRepository_SaveRejectsDuplicate(t *testing.T) {
	repo := newCredentialRepo(t)
	ctx := context.Background()

	credential := &entity.Credential{
		ID:               "cred-1",
		ProposalID:       "p-1",
		UserID:           "pseudonym-a",
		UserPubKey:       []byte("pub"),
		Signature:        []byte("sig"),
		IssuerKeyVersion: 1,
		IssuedAt:         time.Now().UTC(),
	}
	require.NoError(t, repo.Save(ctx, credential))

	duplicate := &entity.Credential{
		ID:               "cred-2",
		ProposalID:       "p-1",
		UserID:           "pseudonym-a",
		UserPubKey:       []byte("pub-2"),
		Signature:        []byte("sig-2"),
		IssuerKeyVersion: 1,
		IssuedAt:         time.Now().UTC(),
	}
	err := repo.Save(ctx, duplicate)
	require.ErrorIs(t, err, repository.ErrDuplicateCredential)
}

// 同一个 user_id 在不同提案下互不影响。
func TestCredentialRepository_SaveAllowsSameUserAcrossProposals(t *testing.T) {
	repo := newCredentialRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Save(ctx, &entity.Credential{
		ID:         "cred-1",
		ProposalID: "p-1",
		UserID:     "pseudonym-a",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
		IssuedAt:   time.Now().UTC(),
	}))
	require.NoError(t, repo.Save(ctx, &entity.Credential{
		ID:         "cred-2",
		ProposalID: "p-2",
		UserID:     "pseudonym-a",
		UserPubKey: []byte("pub"),
		Signature:  []byte("sig"),
		IssuedAt:   time.Now().UTC(),
	}))

	credential, err := repo.GetByProposalAndUserID(ctx, "p-1", "pseudonym-a")
	require.NoError(t, err)
	require.Equal(t, "cred-1", credential.ID)
}
