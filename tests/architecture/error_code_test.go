// 本文件钉住**错误码集合**这条跨语言契约。
//
// 背景：错误码是服务端与门户之间的**机器可读接口**。服务端返回
// `{ error, code }`，门户按 `code` 决定给用户看什么话。服务端加了一个码、
// 门户不认识，用户看到的就是一句英文原文 —— 而且**不会有任何报错**：
// 两边各自的自测都是绿的。
//
// 这正是「两边各拿自己的常量自证是测不出这类问题」的典型：门户的
// `error-messages.ts` 只做 JSON 解析与转发，服务端的 `MapError` 只管
// 映射，谁也不知道对方认不认识新码。
//
// 所以这里**双向**比对：Go 侧 `MapError` 返回的码集合 ↔ TS 侧
// `PortalErrorCode` 联合类型的成员集合。改了一边不改另一边，这里就红。
//
// 与字段名那条门禁（api_contract_test.go）是同一套思路：契约要冻结，
// 冻结要靠**可执行的断言**，不能只靠文档。
package architecture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestErrorCodesMatchAcrossLanguages 断言错误码集合两边一致。
//
// Go 侧：`internal/infrastructure/http/middleware/error_handler.go` 的
// `MapError`（领域错误 → 码）与 `RateLimitedCode`（限流码）。
// TS 侧：`web/src/lib/error-messages.ts` 的 `PortalErrorCode`。
func TestErrorCodesMatchAcrossLanguages(t *testing.T) {
	root := repoRoot(t)

	goCodes := goErrorCodes(t, filepath.Join(
		root, "internal", "infrastructure", "http", "middleware", "error_handler.go",
	))
	tsCodes := tsUnionMembers(t, filepath.Join(
		root, "web", "src", "lib", "error-messages.ts",
	), "PortalErrorCode")

	require.NotEmpty(t, goCodes, "在 Go 侧找不到错误码")
	require.NotEmpty(t, tsCodes, "在 TS 侧找不到错误码")

	sort.Strings(goCodes)
	sort.Strings(tsCodes)
	require.Equal(t, goCodes, tsCodes,
		"错误码集合两边必须一致 —— 服务端加了码而门户不认识，用户看到的就是英文原文")
}

// TestErrorCodesHaveChineseMessages 断言每个错误码都有中文文案。
//
// 这条是门户侧的底线：错误文案是给用户看的，不能是后端的英文原文。
// TS 侧另有一条自测（web/src/lib/error-messages.test.ts）钉住同一件事。
func TestErrorCodesHaveChineseMessages(t *testing.T) {
	root := repoRoot(t)

	source := readSource(t, filepath.Join(root, "web", "src", "lib", "error-messages.ts"))

	// 取 `CODE_MESSAGES` 这张表的字面量：形如 `RATE_LIMITED: '请求太频繁了…'`
	matches := regexp.MustCompile(`(?m)^\s*([A-Z_]+):\s*'([^']*)'`).FindAllStringSubmatch(source, -1)
	require.NotEmpty(t, matches, "在 error-messages.ts 里找不到错误码文案表")

	chinese := regexp.MustCompile(`[\p{Han}]`)
	for _, match := range matches {
		code, message := match[1], match[2]
		require.True(t, chinese.MatchString(message),
			"错误码 %s 的文案 %q 里没有中文 —— 用户看到的是后端原文", code, message)
	}
}

// goErrorCodes 取一个 Go 文件里定义的全部错误码。
//
// 两个来源：
//  1. `MapError` 函数体里的字符串字面量 —— 领域错误映射出的码；
//  2. 包级常量 `RateLimitedCode` —— 限流码（它不经过 MapError：
//     限流是传输层的保护措施，不是领域错误）。
func goErrorCodes(t *testing.T, path string) []string {
	t.Helper()

	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, nil, 0)
	require.NoError(t, err)

	codes := map[string]bool{}
	for _, decl := range parsed.Decls {
		switch typed := decl.(type) {
		case *ast.FuncDecl:
			if typed.Name.Name != "MapError" || typed.Body == nil {
				continue
			}
			ast.Inspect(typed.Body, func(node ast.Node) bool {
				literal, ok := node.(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					return true
				}
				value := strings.Trim(literal.Value, `"`)
				if value != "" {
					codes[value] = true
				}
				return true
			})
		case *ast.GenDecl:
			if typed.Tok != token.CONST {
				continue
			}
			for _, spec := range typed.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, ident := range valueSpec.Names {
					if ident.Name != "RateLimitedCode" || index >= len(valueSpec.Values) {
						continue
					}
					literal, ok := valueSpec.Values[index].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						continue
					}
					codes[strings.Trim(literal.Value, `"`)] = true
				}
			}
		}
	}

	out := make([]string, 0, len(codes))
	for code := range codes {
		out = append(out, code)
	}
	return out
}

// tsUnionMembers 取一个 TS 联合类型里的全部字符串字面量成员。
//
// 形如：
//
//	export type PortalErrorCode =
//	  | 'INVALID_REQUEST'
//	  | 'DOUBLE_VOTING';
func tsUnionMembers(t *testing.T, path string, typeName string) []string {
	t.Helper()

	source := readSource(t, path)

	marker := "export type " + typeName
	start := strings.Index(source, marker)
	require.GreaterOrEqual(t, start, 0, "在 %s 里找不到类型 %s", path, typeName)

	rest := source[start:]
	end := strings.Index(rest, ";")
	require.GreaterOrEqual(t, end, 0, "类型 %s 的定义没有结束", typeName)

	matches := regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(rest[:end], -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		out = append(out, match[1])
	}
	return out
}
