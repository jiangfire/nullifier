package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"sort"
	"strings"

	domaincrypto "anonvote/internal/domain/crypto"
	"anonvote/internal/infrastructure/crypto/eligibility"

	"github.com/consensys/gnark-crypto/ecc/bn254/fr"
)

// rosterEntry 是一套资格名单的服务端侧全部材料：**根 + 验证密钥 + 树深**。
//
// 没有名单本身、没有成员包 —— 它能验证「这个资格在这棵树里」，却无法把
// 资格对应到任何成员。三项必须成套：换名单要一起换，单项不匹配时证明必然
// 验不过。
type rosterEntry struct {
	name     string
	root     string
	depth    int
	verifier *eligibility.Verifier
}

// EligibilityService 是资格证明的服务端侧实现。
//
// 它持有一到多套名单的根与验证密钥（按名字索引）；与选票证明不同，这里
// **没有**「跳过验证」的降级模式：验签密钥缺失就等于服务端无法履行一人一票
// 的承诺，那必须在启动时失败，而不是悄悄放行。
type EligibilityService struct {
	rosters map[string]*rosterEntry
	// defaultName 是空 scope 的解析结果：名为 default 的名单；只配置了一套
	// 名单时是那一套。两者都不满足时为空 —— 空 scope 解析失败（fail-closed）。
	defaultName string
}

var _ domaincrypto.EligibilityService = (*EligibilityService)(nil)

// NewEligibilityService 造一个单名单的资格校验服务（root 即默认名单域）。
//
// root 与验证密钥要么都给、要么都不给：只给根没有验证密钥等于「不校验」，只给验证
// 密钥没有根则无从验证。都不给表示**未开启**资格校验（开发环境的降级形态）。
// depth 是名单树的深度 —— root / 验证密钥 / 深度是**一套**，换名单要成套换
// （v0.4.0 起深度是部署决策，不再是编译期常量）。
func NewEligibilityService(root string, verifyingKeyPath string, depth int) (*EligibilityService, error) {
	spec := RosterSpec{Name: "default", Root: root, VerifyingKeyPath: verifyingKeyPath, Depth: depth}
	if strings.TrimSpace(root) == "" && strings.TrimSpace(verifyingKeyPath) == "" {
		// 都没配 = 未开启资格校验（开发环境的降级形态）：传一个全空条目，
		// newEligibilityService 会把它解析成「未开启」而不是报错。
		spec = RosterSpec{}
	}
	return newEligibilityService([]RosterSpec{spec})
}

// RosterSpec 是一套名单的声明：名字、根、验证密钥与树深。
type RosterSpec struct {
	// Name 是名单域的名字。提案通过它绑定名单；名为 default 的一套同时充当
	// 未显式绑定名单的提案的默认名单。
	Name string
	// Root 是名单的 Merkle 树根（十六进制域元素）。
	Root string
	// VerifyingKeyPath 是该名单对应深度电路的验证密钥路径。
	VerifyingKeyPath string
	// Depth 是名单树的深度。
	Depth int
}

// NewEligibilityServiceFromSpecs 造一个多名单的资格校验服务。
//
// 每套名单都要 root / 密钥 / 深度齐全；名字不能重复。没有任何合法名单时返回
// 「未开启」的服务（开发环境的降级形态）。
func NewEligibilityServiceFromSpecs(specs []RosterSpec) (*EligibilityService, error) {
	return newEligibilityService(specs)
}

func newEligibilityService(specs []RosterSpec) (*EligibilityService, error) {
	rosters := make(map[string]*rosterEntry, len(specs))
	var loaded int
	for _, spec := range specs {
		name := strings.TrimSpace(spec.Name)
		root := strings.TrimSpace(spec.Root)
		keyPath := strings.TrimSpace(spec.VerifyingKeyPath)
		if name == "" && root == "" && keyPath == "" {
			// 全空的条目表示「这一套没配」，直接跳过 —— 单名单的旧入口
			// （NewEligibilityService）在未开启资格校验时就是这样传进来的。
			continue
		}
		loaded++
		switch {
		case name == "":
			return nil, errors.New("eligibility roster name is required")
		case root == "":
			return nil, fmt.Errorf("eligibility roster %q: root is required", name)
		case keyPath == "":
			return nil, fmt.Errorf("eligibility roster %q: verifying key path is required", name)
		}
		if err := eligibility.ValidateTreeDepth(spec.Depth); err != nil {
			return nil, fmt.Errorf("eligibility roster %q: %w", name, err)
		}
		if _, exists := rosters[name]; exists {
			return nil, fmt.Errorf("eligibility roster %q is configured more than once", name)
		}
		if _, err := eligibility.DecodeField(root); err != nil {
			return nil, fmt.Errorf("eligibility roster %q: invalid root: %w", name, err)
		}
		verifier, err := eligibility.NewVerifier(keyPath)
		if err != nil {
			return nil, fmt.Errorf("eligibility roster %q: %w", name, err)
		}
		rosters[name] = &rosterEntry{name: name, root: root, depth: spec.Depth, verifier: verifier}
	}

	service := &EligibilityService{rosters: rosters}
	switch {
	case len(rosters) == 0:
		// 未开启：保留空服务（Enabled() == false）。
	case rosters["default"] != nil:
		service.defaultName = "default"
	case len(rosters) == 1:
		for name := range rosters {
			service.defaultName = name
		}
	}
	return service, nil
}

// Enabled 报告资格校验是否开启。
func (s *EligibilityService) Enabled() bool {
	return s != nil && len(s.rosters) > 0
}

// RosterFor 返回某个名单域的树根与树深。
func (s *EligibilityService) RosterFor(scope string) (string, int, error) {
	entry, err := s.resolve(scope)
	if err != nil {
		return "", 0, err
	}
	return entry.root, entry.depth, nil
}

// ScopeKnown 报告某个名单域是否存在。
func (s *EligibilityService) ScopeKnown(scope string) bool {
	if strings.TrimSpace(scope) == "" {
		return true
	}
	return s.rosters[strings.TrimSpace(scope)] != nil
}

// NewChallenge 生成一个一次性的领取挑战。
//
// 挑战是 32 字节随机值**归约进标量域**后的规范编码：随机字节有约 2^-32 的
// 概率超过模数，直接当域元素用会出现「同一个值两种字节表示」。
func (s *EligibilityService) NewChallenge() (string, error) {
	if !s.Enabled() {
		return "", errors.New("eligibility verification is not configured")
	}
	raw := make([]byte, eligibility.FieldSize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("read random challenge: %w", err)
	}
	var element fr.Element
	element.SetBytes(raw)
	return eligibility.EncodeField(element), nil
}

// VerifyFor 校验一份资格证明是否与给定名单域的公开输入匹配。
func (s *EligibilityService) VerifyFor(scope string, proof []byte, statement domaincrypto.EligibilityStatement) error {
	entry, err := s.resolve(scope)
	if err != nil {
		return err
	}
	built, err := eligibility.BuildStatement(
		entry.root,
		statement.ProposalID,
		statement.Challenge,
		statement.QualificationID,
	)
	if err != nil {
		return fmt.Errorf("build eligibility statement: %w", err)
	}
	return entry.verifier.Verify(proof, built)
}

// resolve 按名单域名字找名单。
//
// 空名单域（未显式绑定）取默认名单：名为 default 的那套，或唯一的一套。
// 都不满足时报错 —— 多名单并存且没有默认名单时，提案必须显式绑定。
func (s *EligibilityService) resolve(scope string) (*rosterEntry, error) {
	if !s.Enabled() {
		return nil, errors.New("eligibility verification is not configured")
	}
	name := strings.TrimSpace(scope)
	if name == "" {
		if s.defaultName == "" {
			return nil, fmt.Errorf(
				"no default eligibility roster: %s",
				strings.Join(s.rosterNames(), ", "),
			)
		}
		name = s.defaultName
	}
	entry, exists := s.rosters[name]
	if !exists {
		return nil, fmt.Errorf("unknown eligibility roster %q (configured: %s)",
			name, strings.Join(s.rosterNames(), ", "))
	}
	return entry, nil
}

// rosterNames 返回排好序的名单域名字（错误信息与日志才稳定）。
func (s *EligibilityService) rosterNames() []string {
	names := make([]string, 0, len(s.rosters))
	for name := range s.rosters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
