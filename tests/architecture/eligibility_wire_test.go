// 本文件钉住**资格证明的跨语言线路**。
//
// 背景：资格证明的密码学全在 Go 侧（WASM 模块），门户这一侧只做 JSON 解析
// 与转发。于是最容易出错的不是数学，而是**字符串**：字段名、静态资源名、
// WASM API 名。这些一旦两边错开，表现往往不是报错，而是「资格包无效」
// 或者浏览器里一个 404。
//
// 因此这里两边各钉一条：Go 侧的 struct tag / api.Set 的键，与 TS 侧的接口
// 字段 / 调用名。改了一边不改另一边，这里就会红。
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestEligibilityMemberPackageFieldsMatch 断言成员包的字段名两边一致。
//
// Go 侧：`internal/infrastructure/crypto/eligibility/merkle.go` 的
// `MemberPackage` 的 json tag（离线名单工具产出的 JSON 就是它）。
//
// TS 侧：`web/src/lib/wasm-prover-protocol.ts` 的 `EligibilityMemberPackage`。
func TestEligibilityMemberPackageFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "internal", "infrastructure", "crypto", "eligibility", "merkle.go"),
		"MemberPackage",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "wasm-prover-protocol.ts"),
		"EligibilityMemberPackage",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"成员包的字段名两边必须一致 —— Go 侧改 json tag 就要同步改 TS 侧")
}

// TestEligibilityProveRequestFieldsMatch 断言「出资格证明」的请求体字段名
// 两边一致。
//
// Go 侧：`cmd/ballot-wasm/main.go` 的 `eligibilityProveRequest`。
// TS 侧：`web/src/lib/wasm-prover-protocol.ts` 的 `EligibilityProveRequest`。
func TestEligibilityProveRequestFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "cmd", "ballot-wasm", "main.go"),
		"eligibilityProveRequest",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "wasm-prover-protocol.ts"),
		"EligibilityProveRequest",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"资格证明请求体的字段名两边必须一致")
}

// TestEligibilityWasmApiNamesMatch 断言 WASM 模块的 API 名两边一致。
//
// Go 侧注册：`api.Set("initEligibility", ...)`。
// TS 侧调用：`api.initEligibility(...)`。
func TestEligibilityWasmApiNamesMatch(t *testing.T) {
	root := repoRoot(t)

	goSource := readSource(t, filepath.Join(root, "cmd", "ballot-wasm", "main.go"))
	tsSource := readSource(t, filepath.Join(root, "web", "src", "workers", "ballot-prover.worker.ts"))

	for _, name := range []string{"initEligibility", "proveEligibility"} {
		require.Contains(t, goSource, "api.Set(\""+name+"\"",
			"WASM 模块必须注册 %s", name)
		require.Contains(t, tsSource, "api."+name+"(",
			"Worker 必须调用 %s", name)
	}
}

// TestEligibilityProvingKeyAssetNameMatches 断言资格 proving key 的静态资源
// 名两边一致。
//
// 一边是 Makefile 拷过去的文件名，一边是门户请求的文件名 —— 对不上就是
// 浏览器里一个 404，出证明时才会暴露。
func TestEligibilityProvingKeyAssetNameMatches(t *testing.T) {
	root := repoRoot(t)

	makefile := readSource(t, filepath.Join(root, "Makefile"))
	tsSource := readSource(t, filepath.Join(root, "web", "src", "lib", "eligibility-prover.ts"))

	require.Contains(t, makefile, "web/public/eligibility-proving.key",
		"Makefile 必须把资格 proving key 发布到这个名字")
	require.Contains(t, tsSource, "'eligibility-proving.key'",
		"门户必须请求这个名字的静态资源")
}

// jsonTagFields 取某个结构体类型上的 json tag 名（按声明顺序）。
//
// 用 go/ast 而不是直接 import：`cmd/ballot-wasm/main.go` 带
// `js && wasm` 构建标签，在宿主平台上根本编不过。
func jsonTagFields(t *testing.T, path string, typeName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)

	var names []string
	for _, decl := range parsed.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != typeName {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				name := jsonTagName(field)
				if name == "" {
					continue
				}
				names = append(names, name)
			}
		}
	}
	require.NotEmpty(t, names, "在 %s 里找不到类型 %s", path, typeName)
	return names
}

// tsInterfaceFields 取 TS 接口里的字段名（按声明顺序）。
func tsInterfaceFields(t *testing.T, path string, interfaceName string) []string {
	t.Helper()
	source := readSource(t, path)

	var names []string
	inInterface := false
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if !inInterface {
			if strings.HasPrefix(trimmed, "export interface "+interfaceName+" ") {
				inInterface = true
			}
			continue
		}
		if trimmed == "}" {
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "//") ||
			strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*") {
			continue
		}
		name := trimmed
		if index := strings.Index(name, ":"); index >= 0 {
			name = name[:index]
		}
		name = strings.TrimSuffix(strings.TrimSpace(name), "?")
		if !isIdentifier(name) {
			continue
		}
		names = append(names, name)
	}
	require.NotEmpty(t, names, "在 %s 里找不到接口 %s", path, interfaceName)
	return names
}

// isIdentifier 判断是不是一个合法的标识符。
func isIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char == '_':
		case char >= '0' && char <= '9' && index > 0:
		default:
			return false
		}
	}
	return true
}

// readSource 读一个文件的内容。
func readSource(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(raw)
}
