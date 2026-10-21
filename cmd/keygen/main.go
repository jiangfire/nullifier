// Command keygen 生成选票电路的 Groth16 密钥。
//
// verifying key 部署到服务端；proving key 只在客户端侧使用（浏览器 WASM
// 或 CLI），不能进仓库、不能进服务端镜像。
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"anonvote/internal/infrastructure/crypto/ballot"
)

const (
	// verifyingKeyFileName 是验证密钥的文件名。
	verifyingKeyFileName = "verifying.key"
	// provingKeyFileName 是证明密钥的文件名。
	provingKeyFileName = "proving.key"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run 解析参数并生成密钥，返回进程退出码。main 里只做 os.Exit，
// 逻辑放在这里是为了让生成流程能进单元测试。
func run(args []string) int {
	fs := flag.NewFlagSet("nullifier-keygen", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	dir := fs.String("dir", "./keys", "directory to write the keys into")
	force := fs.Bool("force", false, "overwrite existing key files")
	verifyingPath := fs.String("verifying-out", "", "path of the verifying key (default <dir>/"+verifyingKeyFileName+")")
	provingPath := fs.String("proving-out", "", "path of the proving key (default <dir>/"+provingKeyFileName+")")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, "用法: nullifier-keygen [-dir <目录>] [-force] [-verifying-out <路径>] [-proving-out <路径>]")
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	verifying := *verifyingPath
	if verifying == "" {
		verifying = filepath.Join(*dir, verifyingKeyFileName)
	}
	proving := *provingPath
	if proving == "" {
		proving = filepath.Join(*dir, provingKeyFileName)
	}

	if err := generateKeys(verifying, proving, *force); err != nil {
		fmt.Fprintf(os.Stderr, "生成密钥失败: %v\n", err)
		return 1
	}

	summary, err := os.ReadFile(verifying)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取验证密钥失败: %v\n", err)
		return 1
	}
	digest := sha256.Sum256(summary)
	fmt.Fprintf(os.Stdout, "verifying key: %s\n", verifying)
	fmt.Fprintf(os.Stdout, "verifying key fingerprint (sha256): %s\n", hex.EncodeToString(digest[:]))
	fmt.Fprintf(os.Stdout, "proving key: %s （仅客户端使用，不要提交到仓库）\n", proving)
	return 0
}

// generateKeys 生成密钥并落盘。
func generateKeys(verifyingPath, provingPath string, force bool) error {
	if !force {
		for _, path := range []string{verifyingPath, provingPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use -force to overwrite)", path)
			}
		}
	}

	provingKey, verifyingKey, err := ballot.GenerateKeys()
	if err != nil {
		return err
	}
	if err := writeKeyFile(verifyingPath, verifyingKey, 0o644); err != nil {
		return err
	}
	// proving key 是秘密材料：只有客户端能读。
	return writeKeyFile(provingPath, provingKey, 0o600)
}

// writeKeyFile 写密钥文件，权限收紧到只允许所有者读写（公开的验证密钥
// 例外，按 0644 落盘以便服务端多副本读取）。
func writeKeyFile(path string, data []byte, mode os.FileMode) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
