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

// TestRunGeneratesEligibilityKeys 覆盖第二套电路：资格电路的密钥文件名与
// 选票那套不同，两套不能互相覆盖。
func TestRunGeneratesEligibilityKeys(t *testing.T) {
	dir := t.TempDir()

	require.Equal(t, 0, run([]string{"-circuit", "eligibility", "-dir", dir}))

	verifying, err := os.ReadFile(filepath.Join(dir, eligibilityVerifyingKeyFileName))
	require.NoError(t, err)
	require.NotEmpty(t, verifying)

	proving, err := os.ReadFile(filepath.Join(dir, eligibilityProvingKeyFileName))
	require.NoError(t, err)
	require.NotEmpty(t, proving)

	// 选票那套的文件名不该被资格电路覆盖。
	_, err = os.Stat(filepath.Join(dir, verifyingKeyFileName))
	require.True(t, os.IsNotExist(err))
}

// TestRunRejectsUnknownCircuit 未知电路直接报错，不静默生成选票那套。
func TestRunRejectsUnknownCircuit(t *testing.T) {
	require.Equal(t, 2, run([]string{"-circuit", "nope", "-dir", t.TempDir()}))
}

// TestDefaultKeyNames 钉住两套电路的默认文件名。
func TestDefaultKeyNames(t *testing.T) {
	verifying, proving, err := defaultKeyNames(circuitBallot)
	require.NoError(t, err)
	require.Equal(t, verifyingKeyFileName, verifying)
	require.Equal(t, provingKeyFileName, proving)

	verifying, proving, err = defaultKeyNames(circuitEligibility)
	require.NoError(t, err)
	require.Equal(t, eligibilityVerifyingKeyFileName, verifying)
	require.Equal(t, eligibilityProvingKeyFileName, proving)

	_, _, err = defaultKeyNames("nope")
	require.Error(t, err)
}
