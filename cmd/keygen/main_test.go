package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunGeneratesKeys(t *testing.T) {
	dir := t.TempDir()
	verifyingPath := filepath.Join(dir, "verifying.key")
	provingPath := filepath.Join(dir, "proving.key")

	code := run([]string{"-verifying-out", verifyingPath, "-proving-out", provingPath})
	require.Equal(t, 0, code)

	verifying, err := os.ReadFile(verifyingPath)
	require.NoError(t, err)
	require.NotEmpty(t, verifying)

	proving, err := os.ReadFile(provingPath)
	require.NoError(t, err)
	require.NotEmpty(t, proving)

	// 证明密钥是秘密材料：权限必须收紧。
	// Windows 没有 POSIX 权限位，这一条只在类 Unix 平台上检查。
	if runtime.GOOS != "windows" {
		info, err := os.Stat(provingPath)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestRunRefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	verifyingPath := filepath.Join(dir, "verifying.key")
	provingPath := filepath.Join(dir, "proving.key")

	require.Equal(t, 0, run([]string{"-verifying-out", verifyingPath, "-proving-out", provingPath}))
	require.Equal(t, 1, run([]string{"-verifying-out", verifyingPath, "-proving-out", provingPath}))
	require.Equal(t, 0, run([]string{"-force", "-verifying-out", verifyingPath, "-proving-out", provingPath}))
}

func TestRunHelp(t *testing.T) {
	require.Equal(t, 0, run([]string{"-h"}))
}

func TestRunRejectsBadFlag(t *testing.T) {
	require.Equal(t, 2, run([]string{"-unknown"}))
}
