// 本文件钉住**冻结的接口契约**（v1）。
//
// 背景：v0.3.0 的承诺是「接口不再动」。冻结不是一句承诺 —— 它是三样东西：
//  1. `docs/api-contract-v1.md` 逐字段写清线格式；
//  2. 本文件把它变成**可执行的门禁**；
//  3. 密文格式定版（版本号与长度不再变）。
//
// 最容易出错的不是密码学，而是**字符串**：字段名、端点路径、格式号。
// 两边各拿自己的常量自证是测不出这类问题的 —— 门户这一侧只做 JSON 解析与转发，
// 字段名一旦错开，表现往往不是报错，而是「服务端说字段缺失」。
//
// 因此这里**双向**比对：Go 侧的 struct tag / 常量 ↔ TS 侧的接口字段 / 常量。
// 改了一边不改另一边，这里就会红。
//
// 契约的口径见 [`docs/api-contract-v1.md`](../../docs/api-contract-v1.md)。
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// 冻结的端点路径。
//
// 用 `{proposal_id}` 这样的**规范占位符**写 —— Go 侧注册的是 `:proposal_id`，
// TS 侧是 `${encodeURIComponent(proposalId)}`，两边归一之后才能比对。
// 与 `docs/api-contract-v1.md` 逐条对应。改这里等于改契约 —— 那要走下一个 minor 版本。
var frozenVoteSubmitPath = "/api/v1/proposals/{proposal_id}/votes"

var frozenCredentialPaths = []string{
	"/api/v1/proposals/{proposal_id}/credentials/eligibility-challenge",
	"/api/v1/proposals/{proposal_id}/credentials/claim",
	"/api/v1/proposals/{proposal_id}/credentials/blind-sign",
	"/api/v1/proposals/{proposal_id}/credentials/attest",
}

// TestVoteSubmitRequestFieldsMatch 断言投票提交请求体的字段名两边一致。
//
// Go 侧：`internal/infrastructure/http/handler.go` 的 `submitVotePayload`。
// TS 侧：`web/src/lib/api-contract.ts` 的 `VoteSubmitRequest`。
//
// ⚠️ 这条同时钉住了一件事：`proposal_id` **不在**请求体里 ——
// 它只从路径来（见 T23 的接口收敛）。
func TestVoteSubmitRequestFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "internal", "infrastructure", "http", "handler.go"),
		"submitVotePayload",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "api-contract.ts"),
		"VoteSubmitRequest",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"投票提交请求体的字段名两边必须一致 —— Go 侧改 json tag 就要同步改 TS 侧")

	require.NotContains(t, goFields, "proposal_id",
		"提案 ID 只从路径来 —— 请求体里再放一份，两个来源可能打架")
	require.NotContains(t, goFields, "selected_option",
		"明文选项不许上服务端 —— 那会抵消「选票是加密的」这个属性")
}

// TestCredentialClaimRequestFieldsMatch 断言凭证领取请求体的字段名两边一致。
//
// Go 侧：`claimCredentialPayload`。TS 侧：`CredentialClaimRequest`。
func TestCredentialClaimRequestFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "internal", "infrastructure", "http", "handler.go"),
		"claimCredentialPayload",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "api-contract.ts"),
		"CredentialClaimRequest",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"凭证领取请求体的字段名两边必须一致")
}

// TestCredentialBlindSignRequestFieldsMatch 断言盲签名请求体的字段名两边一致。
func TestCredentialBlindSignRequestFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "internal", "infrastructure", "http", "handler.go"),
		"blindSignPayload",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "api-contract.ts"),
		"CredentialBlindSignRequest",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"盲签名请求体的字段名两边必须一致")
}

// TestCredentialAttestRequestFieldsMatch 断言 attest 请求体的字段名两边一致。
func TestCredentialAttestRequestFieldsMatch(t *testing.T) {
	root := repoRoot(t)

	goFields := jsonTagFields(
		t,
		filepath.Join(root, "internal", "infrastructure", "http", "handler.go"),
		"attestCredentialPayload",
	)
	tsFields := tsInterfaceFields(
		t,
		filepath.Join(root, "web", "src", "lib", "api-contract.ts"),
		"CredentialAttestRequest",
	)

	sort.Strings(goFields)
	sort.Strings(tsFields)
	require.Equal(t, goFields, tsFields,
		"attest 请求体的字段名两边必须一致")
}

// TestVoteSubmitRouteIsFrozen 断言投票提交的端点路径是冻结的那一条。
//
// Go 侧：注册路由（`:proposal_id`）。TS 侧：`apiPaths.voteSubmit`（`${...}`）。
// 两边归一成 `{proposal_id}` 之后比对。
//
// 旧路径 `POST /api/v1/votes` 在 v0.3.0 被移除 —— 端点不存在就该是 404，
// 而不是 400（400 会让人以为「参数不对」，其实端点已经搬走了）。
func TestVoteSubmitRouteIsFrozen(t *testing.T) {
	root := repoRoot(t)

	goPaths := registeredRoutes(t, filepath.Join(root, "internal", "infrastructure", "http", "handler.go"))
	tsPaths := templateLiteralPaths(t, filepath.Join(root, "web", "src", "lib", "api-contract.ts"))

	require.Contains(t, goPaths, frozenVoteSubmitPath,
		"服务端必须注册冻结的投票提交端点")
	require.Contains(t, tsPaths, frozenVoteSubmitPath,
		"TS 侧的冻结契约必须声明同一条路径")

	require.NotContains(t, goPaths, "/api/v1/votes",
		"旧路径 POST /api/v1/votes 已经移除 —— 提案 ID 走路径才只有一个来源")
}

// TestCredentialRoutesAreFrozen 断言凭证领取的四个端点路径两边一致。
func TestCredentialRoutesAreFrozen(t *testing.T) {
	root := repoRoot(t)

	goPaths := registeredRoutes(t, filepath.Join(root, "internal", "infrastructure", "http", "handler.go"))
	tsPaths := templateLiteralPaths(t, filepath.Join(root, "web", "src", "lib", "api-contract.ts"))

	for _, path := range frozenCredentialPaths {
		require.Contains(t, goPaths, path, "服务端必须注册冻结的凭证端点 %s", path)
		require.Contains(t, tsPaths, path, "TS 侧的冻结契约必须声明同一条路径 %s", path)
	}
}

// registeredRoutes 取一个 Go 文件里注册的所有路由，并归一成规范占位符。
//
// gin 的路径参数写作 `:proposal_id`，归一之后是 `{proposal_id}`。
// 路由挂在 `/api/v1` 分组下，这里补上前缀。
func registeredRoutes(t *testing.T, path string) []string {
	t.Helper()

	source := readSource(t, path)
	matches := regexp.MustCompile(`r\.(?:GET|POST|PUT|DELETE)\("([^"]+)"`).FindAllStringSubmatch(source, -1)

	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, normalizeRoutePath("/api/v1"+match[1]))
	}
	require.NotEmpty(t, out, "在 %s 里找不到注册的路由", path)
	return out
}

// templateLiteralPaths 取一个 TS 文件里的所有路径模板字面量，并归一成规范占位符。
//
// 模板写作 `${encodeURIComponent(proposalId)}`，归一之后是 `{proposal_id}`。
func templateLiteralPaths(t *testing.T, path string) []string {
	t.Helper()

	source := readSource(t, path)
	matches := regexp.MustCompile("`(/api/v1[^`]*)`").FindAllStringSubmatch(source, -1)

	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, normalizeRoutePath(match[1]))
	}
	require.NotEmpty(t, out, "在 %s 里找不到路径模板", path)
	return out
}

// normalizeRoutePath 把路由路径归一成规范占位符形式。
//
// gin 的路径参数写作 `:proposal_id` → `{proposal_id}`；
// TS 的模板写作 `${encodeURIComponent(proposalId)}` → `{proposal_id}`。
func normalizeRoutePath(path string) string {
	path = regexp.MustCompile(`:([a-zA-Z_][a-zA-Z0-9_]*)`).ReplaceAllString(path, "{$1}")
	path = regexp.MustCompile(`\$\{[^}]*\}`).ReplaceAllString(path, "{proposal_id}")
	return path
}

// TestCiphertextFormatIsFinalized 断言选票密文的格式**已定版**。
//
// v3 是「不再变」的格式：格式号 0x03、总长 65 字节、点长 32 字节。
// 改这三个数就是破坏性变更（旧密文解不开），必须走下一个 minor 版本。
func TestCiphertextFormatIsFinalized(t *testing.T) {
	root := repoRoot(t)

	goDir := filepath.Join(root, "internal", "infrastructure", "crypto", "ballot")
	tsSource := readSource(t, filepath.Join(root, "web", "src", "lib", "ballot-cipher.ts"))

	goVersion := goIntConst(t, goDir, "CipherVersion")
	tsVersion := tsIntConst(t, tsSource, "ballotCipherVersion")

	require.Equal(t, 3, goVersion,
		"密文格式号已定版为 v3 —— 改它就是破坏性变更（旧密文解不开）")
	require.Equal(t, goVersion, tsVersion,
		"密文格式号两边必须一致 —— 改了任一边，另一边的密文就解不开")

	goSize := goIntConst(t, goDir, "CipherSize")
	tsSize := tsIntConst(t, tsSource, "ballotCipherSize")
	require.Equal(t, goSize, tsSize, "密文总长两边必须一致")

	goPointSize := goIntConst(t, goDir, "PointSize")
	tsPointSize := tsIntConst(t, tsSource, "ballotPointSize")
	require.Equal(t, goPointSize, tsPointSize, "压缩点长度两边必须一致")

	// 总长 = 1 字节版本号 + 两个压缩点。
	require.Equal(t, 1+2*goPointSize, goSize,
		"密文总长必须是「1 字节版本号 + 两个压缩点」")
	require.Equal(t, 65, goSize, "v3 密文总长已定版为 65 字节")
}

// goIntConst 取一个 Go 包里某个常量的整数值。
//
// 用 go/ast 解析常量声明，而不是正则扫字符串 —— 常量表达式可能是
// `1 + 2*PointSize` 这种形式，正则会把它算错。
//
// 常量可能分散在包内的多个文件（`CipherSize` 在 cipher.go，`PointSize`
// 在 group.go），所以整个目录一起扫，跨文件引用才能解开。
func goIntConst(t *testing.T, dir string, name string) int {
	t.Helper()

	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	values := map[string]int{}
	raw := map[string]ast.Expr{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		require.NoError(t, err)

		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, ident := range valueSpec.Names {
					if index >= len(valueSpec.Values) {
						continue
					}
					raw[ident.Name] = valueSpec.Values[index]
				}
			}
		}
	}

	// 跨文件引用（`CipherSize` 引用另一个文件里的 `PointSize`）要求多轮：
	// 文件的扫描顺序不保证被引用的常量先被解出，反复求值直到没有进展。
	for progress := true; progress; {
		progress = false
		for name, expr := range raw {
			if _, done := values[name]; done {
				continue
			}
			evaluated, ok := evalConstExpr(t, expr, values)
			if !ok {
				continue
			}
			values[name] = evaluated
			progress = true
		}
	}

	value, ok := values[name]
	require.True(t, ok, "在 %s 里找不到常量 %s", dir, name)
	return value
}

// evalConstExpr 求一个常量表达式的整数值。
//
// 只支持密文格式用到的运算：整数、标识符（引用另一个常量）、加法与乘法。
// 解析不了的表达式返回 false —— 那时调用方会得到一条明确的失败信息，
// 而不是悄悄算出错的数。
func evalConstExpr(t *testing.T, expr ast.Expr, values map[string]int) (int, bool) {
	t.Helper()

	switch typed := expr.(type) {
	case *ast.BasicLit:
		value, err := strconv.Atoi(strings.Trim(typed.Value, `"`))
		if err != nil {
			return 0, false
		}
		return value, true
	case *ast.Ident:
		value, ok := values[typed.Name]
		return value, ok
	case *ast.BinaryExpr:
		left, leftOK := evalConstExpr(t, typed.X, values)
		right, rightOK := evalConstExpr(t, typed.Y, values)
		if !leftOK || !rightOK {
			return 0, false
		}
		switch typed.Op {
		case token.ADD:
			return left + right, true
		case token.MUL:
			return left * right, true
		case token.SUB:
			return left - right, true
		}
		return 0, false
	case *ast.ParenExpr:
		return evalConstExpr(t, typed.X, values)
	}
	return 0, false
}

// tsIntConst 取一个 TS 常量的整数值。
//
// TS 侧的常量也可能是表达式（`1 + 2 * ballotPointSize`），所以同样要能求值。
func tsIntConst(t *testing.T, source string, name string) int {
	t.Helper()

	values := map[string]int{}
	raw := map[string]string{}
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		for _, prefix := range []string{"export const ", "const "} {
			if !strings.HasPrefix(trimmed, prefix) {
				continue
			}
			rest := trimmed[len(prefix):]
			space := strings.IndexAny(rest, " =")
			if space <= 0 {
				continue
			}
			ident := rest[:space]
			expr := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest[space:]), "="))
			expr = strings.TrimSpace(strings.TrimSuffix(expr, ";"))
			raw[ident] = expr
		}
	}

	// 跨常量引用（`ballotCipherSize` 引用 `ballotPointSize`）要求多轮求值。
	for progress := true; progress; {
		progress = false
		for ident, expr := range raw {
			if _, done := values[ident]; done {
				continue
			}
			evaluated, ok := evalTSExpr(expr, values)
			if !ok {
				continue
			}
			values[ident] = evaluated
			progress = true
		}
	}

	value, ok := values[name]
	require.True(t, ok, "在 TS 源码里找不到常量 %s", name)
	return value
}

// evalTSExpr 求一个 TS 常量表达式的整数值。
//
// 只支持密文格式用到的运算：整数、标识符、加法与乘法。
func evalTSExpr(expr string, values map[string]int) (int, bool) {
	tokens := strings.Fields(strings.ReplaceAll(expr, "*", " * "))
	tokens = strings.Fields(strings.ReplaceAll(strings.Join(tokens, " "), "+", " + "))

	// 表达式形如 `a + b * c`：按乘法优先求值。
	total := 0
	pending := 0
	havePending := false
	operator := "+"
	for _, token := range tokens {
		switch token {
		case "+", "*":
			operator = token
			continue
		}
		value, ok := tsTokenValue(token, values)
		if !ok {
			return 0, false
		}
		switch operator {
		case "*":
			pending *= value
		case "+":
			if havePending {
				total += pending
			}
			pending = value
			havePending = true
		}
	}
	if havePending {
		total += pending
	}
	return total, true
}

// tsTokenValue 求一个 TS 表达式的记号的值（整数或已知的常量）。
func tsTokenValue(token string, values map[string]int) (int, bool) {
	if value, err := strconv.Atoi(token); err == nil {
		return value, true
	}
	value, ok := values[token]
	return value, ok
}
