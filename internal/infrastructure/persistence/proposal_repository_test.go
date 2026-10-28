package persistence

import (
	"context"
	"testing"
	"time"

	"anonvote/internal/domain/entity"

	"github.com/stretchr/testify/require"
)

func TestProposalRepository_ListOpen_OnlyReturnsActiveOpenProposals(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewProposalRepository(db)
	now := time.Now().UTC()
	proposals := []*entity.Proposal{
		{
			ID:        "p-active-1",
			Title:     "active-1",
			Status:    "open",
			Options:   []string{"yes", "no"},
			StartsAt:  now.Add(-2 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-3 * time.Hour),
		},
		{
			ID:        "p-active-2",
			Title:     "active-2",
			Status:    "open",
			StartsAt:  now.Add(-1 * time.Hour),
			EndsAt:    now.Add(1 * time.Hour),
			CreatedAt: now.Add(-2 * time.Hour),
		},
		{
			ID:        "p-expired",
			Title:     "expired",
			Status:    "open",
			StartsAt:  now.Add(-3 * time.Hour),
			EndsAt:    now.Add(-1 * time.Minute),
			CreatedAt: now.Add(-4 * time.Hour),
		},
		{
			ID:        "p-future",
			Title:     "future",
			Status:    "open",
			StartsAt:  now.Add(1 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-1 * time.Hour),
		},
		{
			ID:        "p-closed",
			Title:     "closed",
			Status:    "closed",
			StartsAt:  now.Add(-2 * time.Hour),
			EndsAt:    now.Add(2 * time.Hour),
			CreatedAt: now.Add(-30 * time.Minute),
		},
	}
	for _, proposal := range proposals {
		require.NoError(t, repo.Save(context.Background(), proposal))
	}

	got, err := repo.ListOpen(context.Background())
	require.NoError(t, err)
	require.Len(t, got, 2)
	require.Equal(t, "p-active-1", got[0].ID)
	require.Equal(t, "p-active-2", got[1].ID)

	all, err := repo.ListAll(context.Background())
	require.NoError(t, err)
	require.Len(t, all, 5, "ListAll must include closed, expired and future proposals")
	require.Equal(t, "p-closed", all[0].ID, "newest first")
	require.Equal(t, "p-expired", all[4].ID, "oldest last")
	require.Equal(t, []string{"yes", "no"}, all[3].Options, "options must be hydrated from options_json")
}

// TestProposalRepository_EligibilityScopeRoundTrip 钉住名单域绑定落库：
// 保存时带上名单域、读回来还在 —— 挑战签发与证明校验靠它按提案解析名单。
// （该列由 AutoMigrate 建立；release 模式升级要先跑 `nullifier-server -migrate`。）
func TestProposalRepository_EligibilityScopeRoundTrip(t *testing.T) {
	db, err := NewDB(&DBConfig{
		Driver:   "sqlite",
		Database: ":memory:",
	})
	require.NoError(t, err)
	require.NoError(t, AutoMigrate(db))

	repo := NewProposalRepository(db)
	now := time.Now().UTC()
	original := &entity.Proposal{
		ID:               "p-scoped",
		Title:            "scoped",
		Status:           "open",
		Options:          []string{"yes", "no"},
		StartsAt:         now.Add(-time.Hour),
		EndsAt:           now.Add(time.Hour),
		CreatedAt:        now,
		EligibilityScope: "acme-2026",
	}
	require.NoError(t, repo.Save(context.Background(), original))

	loaded, err := repo.GetByID(context.Background(), "p-scoped")
	require.NoError(t, err)
	require.Equal(t, "acme-2026", loaded.EligibilityScope)

	// 未绑定名单域的提案读回来是空串 —— 空 = 未显式绑定 = 走默认名单。
	plain := &entity.Proposal{
		ID:        "p-plain",
		Title:     "plain",
		Status:    "open",
		Options:   []string{"yes", "no"},
		StartsAt:  now.Add(-time.Hour),
		EndsAt:    now.Add(time.Hour),
		CreatedAt: now,
	}
	require.NoError(t, repo.Save(context.Background(), plain))
	loaded, err = repo.GetByID(context.Background(), "p-plain")
	require.NoError(t, err)
	require.Empty(t, loaded.EligibilityScope)
}
