package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestVoteRepository_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_USER":     "test",
			"POSTGRES_PASSWORD": "test",
			"POSTGRES_DB":       "anonvote",
		},
		WaitingFor: wait.ForListeningPort("5432/tcp"),
	}

	var (
		pgC testcontainers.Container
		err error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("testcontainers panic: %v", r)
			}
		}()
		pgC, err = testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
			ContainerRequest: req,
			Started:          true,
		})
	}()
	if err != nil {
		if isDockerUnavailable(err) {
			t.Skipf("docker unavailable: %v", err)
		}
		require.NoError(t, err)
	}
	defer func() {
		_ = pgC.Terminate(context.Background())
	}()

	host, err := pgC.Host(ctx)
	require.NoError(t, err)

	port, err := pgC.MappedPort(ctx, "5432")
	require.NoError(t, err)

	db, err := persistence.NewDB(&persistence.DBConfig{
		Driver:   "postgres",
		Host:     host,
		Port:     port.Int(),
		Database: "anonvote",
		Username: "test",
		Password: "test",
		SSLMode:  "disable",
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))

	repo := persistence.NewVoteRepository(db)
	vote := &entity.Vote{
		ID:         "vote-pg-1",
		ProposalID: "proposal-1",
		Nullifier:  "null-pg-1",
		Cipher:     []byte("cipher"),
		CreatedAt:  time.Now().UTC(),
	}
	require.NoError(t, repo.Save(ctx, vote))

	exists, err := repo.ExistsByNullifier(ctx, "null-pg-1")
	require.NoError(t, err)
	require.True(t, exists)
}

func isDockerUnavailable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "docker") ||
		strings.Contains(msg, "daemon") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "cannot connect")
}
