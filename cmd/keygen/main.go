// Command keygen 生成 Groth16 密钥。
//
// verifying key 部署到服务端；proving key 只在客户端侧使用（浏览器 WASM
// 或 CLI），不能进仓库、不能进服务端镜像。
//
// 本仓库有**两套**电路，各自的 setup 互相独立：
//
//   - ballot       —— 选票电路（默认）
//   - eligibility —— 资格证明电路（「一人一票」用）
//
// 两套密钥不能互换：公开输入与见证的形状都不一样。
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
	"anonvote/internal/infrastructure/crypto/eligibility"
)

const (
	// verifyingKeyFileName 是验证密钥的文件名。
	verifyingKeyFileName = "verifying.key"
	// provingKeyFileName 是证明密钥的文件名。
	provingKeyFileName = "proving.key"
	// eligibilityVerifyingKeyFileName 是资格电路的验证密钥文件名。
	eligibilityVerifyingKeyFileName = "eligibility-verifying.key"
	// eligibilityProvingKeyFileName 是资格电路的证明密钥文件名。
	eligibilityProvingKeyFileName = "eligibility-proving.key"
)

// circuitKind 是要生成哪一套电路的密钥。
type circuitKind string

const (
	circuitBallot      circuitKind = "ballot"
	circuitEligibility circuitKind = "eligibility"
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
	circuit := fs.String("circuit", string(circuitBallot), "circuit to generate keys for: ballot | eligibility")
	// depth 只对资格电路有意义：选票电路没有名单树，深度是资格名单的
	// Merkle 树深（默认 10 = 存量口径；每深度一套密钥，换深度 = 全套重出）。
	depth := fs.Int("depth", eligibility.DefaultTreeDepth,
		"eligibility list Merkle tree depth (eligibility circuit only)")
	verifyingPath := fs.String("verifying-out", "", "path of the verifying key")
	provingPath := fs.String("proving-out", "", "path of the proving key")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Fprintln(os.Stdout, "用法: nullifier-keygen [-circuit ballot|eligibility] [-depth <资格名单树深>] [-dir <目录>] [-force] [-verifying-out <路径>] [-proving-out <路径>]")
			fs.SetOutput(os.Stdout)
			fs.PrintDefaults()
			return 0
		}
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	kind := circuitKind(*circuit)
	if kind != circuitEligibility && *depth != eligibility.DefaultTreeDepth {
		fmt.Fprintf(os.Stderr, "参数错误: -depth 只对 -circuit eligibility 有意义\n")
		return 2
	}
	if err := eligibility.ValidateTreeDepth(*depth); err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}
	defaultVerifying, defaultProving, err := defaultKeyNames(kind)
	if err != nil {
		fmt.Fprintf(os.Stderr, "参数错误: %v\n", err)
		return 2
	}

	verifying := *verifyingPath
	if verifying == "" {
		verifying = filepath.Join(*dir, defaultVerifying)
	}
	proving := *provingPath
	if proving == "" {
		proving = filepath.Join(*dir, defaultProving)
	}

	if err := generateKeys(kind, *depth, verifying, proving, *force); err != nil {
		fmt.Fprintf(os.Stderr, "生成密钥失败: %v\n", err)
		return 1
	}

	summary, err := os.ReadFile(verifying)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取验证密钥失败: %v\n", err)
		return 1
	}
	digest := sha256.Sum256(summary)
	fmt.Fprintf(os.Stdout, "circuit: %s\n", kind)
	if kind == circuitEligibility {
		fmt.Fprintf(os.Stdout, "tree depth: %d（名单规模上限 %d 人；root / verifying key / depth 要成套部署）\n",
			*depth, 1<<uint(*depth))
	}
	fmt.Fprintf(os.Stdout, "verifying key: %s\n", verifying)
	fmt.Fprintf(os.Stdout, "verifying key fingerprint (sha256): %s\n", hex.EncodeToString(digest[:]))
	fmt.Fprintf(os.Stdout, "proving key: %s （仅客户端使用，不要提交到仓库）\n", proving)
	return 0
}

// defaultKeyNames 返回该电路的默认密钥文件名。
func defaultKeyNames(kind circuitKind) (string, string, error) {
	switch kind {
	case circuitBallot:
		return verifyingKeyFileName, provingKeyFileName, nil
	case circuitEligibility:
		return eligibilityVerifyingKeyFileName, eligibilityProvingKeyFileName, nil
	default:
		return "", "", fmt.Errorf("未知电路 %q（可选：ballot | eligibility）", kind)
	}
}

// generateKeys 生成密钥并落盘。depth 只有资格电路使用。
func generateKeys(kind circuitKind, depth int, verifyingPath, provingPath string, force bool) error {
	if !force {
		for _, path := range []string{verifyingPath, provingPath} {
			if _, err := os.Stat(path); err == nil {
				return fmt.Errorf("%s already exists (use -force to overwrite)", path)
			}
		}
	}

	var provingKey, verifyingKey []byte
	var err error
	switch kind {
	case circuitBallot:
		provingKey, verifyingKey, err = ballot.GenerateKeys()
	case circuitEligibility:
		provingKey, verifyingKey, err = eligibility.GenerateKeys(depth)
	default:
		return fmt.Errorf("未知电路 %q（可选：ballot | eligibility）", kind)
	}
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
