package main

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"anonvote/internal/config"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/infrastructure/persistence"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// testEnv 把服务端要读的环境变量全部指向临时目录。
//
// 不能依赖工作目录下的默认配置：测试跑在 cmd/server 里，读不到仓库根的
// configs/config.yaml；这里显式给全，免得环境里残留的变量影响断言。
func testEnv(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	t.Setenv("GO_ENV", "")
	t.Setenv("DB_DRIVER", "sqlite")
	t.Setenv("DB_PATH", filepath.Join(dir, "anonvote.db"))
	t.Setenv("CRYPTO_ISSUER_KEY_PATH", filepath.Join(dir, "issuer_keys.json"))
	t.Setenv("CRYPTO_VK_PATH", "")
	t.Setenv("SERVER_ADMIN_BEARER_TOKEN", "")
	t.Setenv("SERVER_MODE", "debug")
	t.Setenv("SERVER_ADDRESS", "127.0.0.1:0")
	return dir
}

func TestRun_RejectsUnknownFlag(t *testing.T) {
	require.Equal(t, 2, run([]string{"-nope"}))
}

func TestRun_HelpExitsZero(t *testing.T) {
	require.Equal(t, 0, run([]string{"-h"}))
}

func TestRun_MigrateCreatesSchema(t *testing.T) {
	dir := testEnv(t)

	require.Equal(t, 0, run([]string{"-migrate"}))

	dbPath := filepath.Join(dir, "anonvote.db")
	require.FileExists(t, dbPath)

	db, err := persistence.NewDB(&persistence.DBConfig{Driver: "sqlite", Database: dbPath})
	require.NoError(t, err)
	// 句柄不关的话，Windows 上临时目录清理会因文件被占用而失败。
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	require.True(t, persistence.SchemaReady(db), "migrate 之后表结构必须就绪")
}

func TestRun_ReleaseModeRequiresVerifyingKey(t *testing.T) {
	testEnv(t)
	t.Setenv("SERVER_MODE", "release")

	require.Equal(t, 1, run(nil))
}

func TestRun_ReleaseModeRequiresAdminToken(t *testing.T) {
	testEnv(t)
	t.Setenv("SERVER_MODE", "release")
	t.Setenv("CRYPTO_VK_PATH", filepath.Join(t.TempDir(), "vk.bin"))

	require.Equal(t, 1, run(nil))
}

// TestServe_ServesHealthzAndShutsDown 是服务端的冒烟测试：真的把服务拉起来，
// 打一次 /healthz，再关掉。中间任一环断掉都会在这里暴露。
func TestServe_ServesHealthzAndShutsDown(t *testing.T) {
	testEnv(t)
	// 冒烟测试只打 /healthz，不碰业务表，所以用内存库：省掉文件句柄
	// 与临时目录清理的麻烦。
	t.Setenv("DB_PATH", ":memory:")

	addrCh := make(chan string, 1)
	stopCh := make(chan struct{})

	original := startServer
	startServer = func(s *httpapi.Server, _ string) error {
		l, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
		go func() { _ = s.Serve(l) }()
		addrCh <- l.Addr().String()
		<-stopCh
		return l.Close()
	}
	t.Cleanup(func() { startServer = original })

	served := make(chan error, 1)
	go func() { served <- serve(false) }()

	var addr string
	select {
	case addr = <-addrCh:
	case err := <-served:
		t.Fatalf("服务在监听之前就退出了: %v", err)
	case <-time.After(30 * time.Second):
		t.Fatal("等待服务监听超时")
	}

	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.JSONEq(t, `{"status":"ok","debug":true,"eligibility":false}`, string(body))

	close(stopCh)
	select {
	case err := <-served:
		// 关停后 listener 会被关掉，Serve 返回的是「use of closed network
		// connection」这类错误，属于预期内的正常退出。
		if err != nil {
			require.ErrorContains(t, err, "closed")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("服务没有在关停信号后退出")
	}
}

// release 模式下 CORS 为空要告警：门户与后端不同源时浏览器会拦掉所有跨域请求，
// 而这种事故在启动日志里看不出来。
func TestWarnIfCORSEmpty(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	warnIfCORSEmpty(logger, &config.Config{GinMode: "release"})
	require.Equal(t, 1, logs.Len())
	require.Contains(t, logs.All()[0].Message, "cors_allowed_origins")

	// 填了 CORS 就不告警。
	warnIfCORSEmpty(logger, &config.Config{
		GinMode:            "release",
		CORSAllowedOrigins: []string{"https://vote.example.com"},
	})
	require.Equal(t, 1, logs.Len())

	// 非 release 模式不告警：开发模式的默认 CORS 本来就够用。
	warnIfCORSEmpty(logger, &config.Config{GinMode: "debug"})
	require.Equal(t, 1, logs.Len())
}
