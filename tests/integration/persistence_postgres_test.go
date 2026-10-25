package integration

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"anonvote/internal/domain/entity"
	"anonvote/internal/domain/repository"
	"anonvote/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func TestVoteRepository_Postgres(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

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

func TestVoteRepository_Postgres_ConcurrentSameUser(t *testing.T) {
	if testing.Short() {
		t.Skip("skip postgres integration in short mode")
	}

	ctx, db, cleanup := setupPostgresDB(t)
	defer cleanup()

	repo := persistence.NewVoteRepository(db)
	proposalID := "proposal-concurrent"
	userHash := "user-hash-concurrent"

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup

	for i := 0; i < 2; i++ {
		vote := &entity.Vote{
			ID:         fmt.Sprintf("vote-pg-concurrent-%d", i+1),
			ProposalID: proposalID,
			Nullifier:  fmt.Sprintf("null-pg-concurrent-%d", i+1),
			Cipher:     []byte("cipher"),
			CreatedAt:  time.Now().UTC(),
		}
		wg.Add(1)
		go func(v *entity.Vote) {
			defer wg.Done()
			<-start
			errs <- repo.SaveWithParticipation(ctx, v, userHash, v.CreatedAt)
		}(vote)
	}

	close(start)
	wg.Wait()
	close(errs)

	success := 0
	duplicateParticipation := 0
	for err := range errs {
		switch {
		case err == nil:
			success++
		case errors.Is(err, repository.ErrDuplicateParticipation):
			duplicateParticipation++
		default:
			require.NoError(t, err)
		}
	}

	require.Equal(t, 1, success)
	require.Equal(t, 1, duplicateParticipation)

	votes, err := repo.GetByProposal(ctx, proposalID)
	require.NoError(t, err)
	require.Len(t, votes, 1)
}

func setupPostgresDB(t *testing.T) (context.Context, *gorm.DB, func()) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

	// 外部 PostgreSQL 优先：本地用 compose 起一个库就能跑，不必依赖 Docker 的
	// testcontainers 拉镜像（离线环境拉不动）。两条路径共用同一份测试代码。
	if settings, ok := externalPostgresSettingsFromEnv(); ok {
		db, cleanup := connectExternalPostgres(t, ctx, settings)
		return ctx, db, func() {
			cleanup()
			cancel()
		}
	}

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
		cancel()
		if isDockerUnavailable(err) {
			// A skipped container test reports the same green as a passing one, so
			// letting CI skip here would silently stop covering PostgreSQL. Only
			// allow the skip outside CI, where Docker is genuinely optional.
			if isCI() {
				require.NoErrorf(t, err,
					"Docker is unavailable; the PostgreSQL integration tests must run in CI, not be skipped")
			}
			t.Skipf("docker unavailable: %v", err)
		}
		require.NoError(t, err)
	}

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

	cleanup := func() {
		_ = pgC.Terminate(context.Background())
		cancel()
	}

	return ctx, db, cleanup
}

// externalPostgresSettings 描述一个外部 PostgreSQL 的连接参数。
type externalPostgresSettings struct {
	Host     string
	Port     int
	User     string
	Password string
	Database string
}

// externalPostgresSettingsFromEnv 从环境变量读外部 PostgreSQL 的连接参数。
//
// 设了 `ANONVOTE_TEST_PG_HOST` 就走外部库（见仓库根的 `docker-compose.yml` 与
// `make pg-test`）；没设就走 testcontainers 拉一个临时容器。
func externalPostgresSettingsFromEnv() (externalPostgresSettings, bool) {
	host := strings.TrimSpace(os.Getenv("ANONVOTE_TEST_PG_HOST"))
	if host == "" {
		return externalPostgresSettings{}, false
	}
	settings := externalPostgresSettings{
		Host:     host,
		Port:     5432,
		User:     "test",
		Password: "test",
		Database: "anonvote",
	}
	if port := strings.TrimSpace(os.Getenv("ANONVOTE_TEST_PG_PORT")); port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			return externalPostgresSettings{}, false
		}
		settings.Port = parsed
	}
	if user := strings.TrimSpace(os.Getenv("ANONVOTE_TEST_PG_USER")); user != "" {
		settings.User = user
	}
	if password := os.Getenv("ANONVOTE_TEST_PG_PASSWORD"); password != "" {
		settings.Password = password
	}
	if database := strings.TrimSpace(os.Getenv("ANONVOTE_TEST_PG_DATABASE")); database != "" {
		settings.Database = database
	}
	return settings, true
}

// connectExternalPostgres 连外部 PostgreSQL，并给这次测试开一个**独立的 schema**。
//
// 每个测试一个 schema：同一��库上反复跑测试不会撞上一次留下的表，跑完直接
// 删掉 schema 即可清理。
func connectExternalPostgres(
	t *testing.T,
	ctx context.Context,
	settings externalPostgresSettings,
) (*gorm.DB, func()) {
	t.Helper()

	schema := fmt.Sprintf("itest_%d", time.Now().UnixNano())
	adminDSN := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		settings.Host,
		settings.Port,
		settings.User,
		settings.Password,
		settings.Database,
	)
	adminDB, err := gorm.Open(gormpostgres.Open(adminDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, adminDB.WithContext(ctx).
		Exec(`CREATE SCHEMA IF NOT EXISTS `+schema).Error)

	dsn := adminDSN + " search_path=" + schema
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	require.NoError(t, persistence.AutoMigrate(db))

	cleanup := func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
		if sqlDB, err := adminDB.DB(); err == nil {
			_, _ = sqlDB.Exec(`DROP SCHEMA IF EXISTS ` + schema + ` CASCADE`)
			_ = sqlDB.Close()
		}
	}
	return db, cleanup
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

// isCI reports whether the suite is running under a CI provider. GitHub Actions
// sets both CI and GITHUB_ACTIONS; GitLab and most others set CI.
func isCI() bool {
	for _, key := range []string{"CI", "GITHUB_ACTIONS"} {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
		case "", "false", "0", "no":
			continue
		default:
			return true
		}
	}
	return false
}
