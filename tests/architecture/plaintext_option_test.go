// Package architecture 放的是「架构不变量」的守卫测试。
//
// 它们不看业务结果，只看**代码里还剩不该剩的东西**。
//
// 背景：能力 A（选票内容对服务端不可知）的验收口径是「服务端代码里**不存在**
// 接收明文选项的字段（grep 可验）」。这条验收以前靠人工 grep —— 人会忘、
// 也会在改动后不再复查。这里把它变成持续可执行的门禁：
//
//  1. HTTP 层的请求体结构里不允许出现明文选项字段（json tag）；
//  2. usecase 层不允许出现第二个「接受明文选项」的入口；
//  3. 每一条被豁免的路径，都必须在 release 模式下真的不可达（404）——
//     豁免表因此是**自证**的：加一条豁免就得同时证明它在生产里是死的。
//
// 豁免表现在只有一条：debug 模式的 `ballot-draft`（服务端代加密的联调接口）。
// 路线图 §5 的 No-Go #3 正在决定这条路径是「删掉」还是「接受它只在 debug
// 模式存在」—— 无论哪种决定，改的都是下面这张表，而不是这条不变量。
//
// 扫描范围说明：只扫 `internal/`（服务端）。`cmd/` 里也有明文选项字段
// （`cmd/cli` 的 ballot-draft 命令、`cmd/ballot-wasm` 的 prove 请求），
// 但那些是**客户端**在说话，不是服务端在收 —— 能力 A 管的是后者。
package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"anonvote/internal/config"
	httpapi "anonvote/internal/infrastructure/http"
	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// plaintextOptionJSONNames 是「明文选项」在请求体里的字段名集合。
//
// 刻意**不含** `options`（复数）：那是提案定义里的候选项列表，由运营方在
// 建提案时给出，与「某个投票人选了哪个」无关。
var plaintextOptionJSONNames = map[string]struct{}{
	"option":          {},
	"selected_option": {},
	"option_index":    {},
	"choice":          {},
	"selected_choice": {},
	"vote_option":     {},
	"ballot_option":   {},
}

// plaintextOptionFieldAllowlist：请求体里的明文选项字段豁免表。
//
// key 是「相对仓库根的文件路径:结构体名.字段名」，value 是豁免理由。
var plaintextOptionFieldAllowlist = map[string]string{
	"internal/infrastructure/http/handler.go:prepareBallotDraftPayload.Option": "debug 模式的 ballot-draft 端点：" +
		"服务端代加密只用于开发联调，release 模式 404（见 TestPlaintextOptionPathsAreUnreachableInReleaseMode）",
}

// plaintextOptionEntryPointAllowlist：usecase 层「接受明文选项」的入口豁免表。
//
// key 是「相对仓库根的文件路径:接收者类型.函数名」（普通函数没有接收者）。
// 这张表描述的是**一整条 debug 链**：handler → PrepareBallotDraft →
// EncryptVoteOption → proposalOptionIndex。
var plaintextOptionEntryPointAllowlist = map[string]string{
	"internal/usecase/tally.go:(*TallyUseCase).EncryptVoteOption":  "服务端代加密的核心实现，只应被同一条 debug 链调用",
	"internal/usecase/tally.go:(*TallyUseCase).PrepareBallotDraft": "debug 草稿入口，只应被 debug 门控的 handler 调用",
	"internal/usecase/tally.go:proposalAllowsOption":               "内部工具函数：判断选项是否在提案的候选项列表里",
	"internal/usecase/tally.go:proposalOptionIndex":                "内部工具函数：选项文本 → 选项下标",
}

// plaintextOptionCallSiteAllowlist：debug 链上函数的**调用点**豁免表。
//
// key 是函数名，value 是允许出现调用点的文件（相对仓库根）。
var plaintextOptionCallSiteAllowlist = map[string]map[string]string{
	"PrepareBallotDraft": {
		"internal/infrastructure/http/handler.go": "只有 debug 门控的 handler 能调它",
	},
	"EncryptVoteOption": {
		"internal/usecase/tally.go": "只被同一条 debug 链内部的 PrepareBallotDraft 调用",
	},
}

// TestNoPlaintextOptionFieldInRequestPayloads 覆盖能力 A 的验收 ①：
// 服务端的请求体结构里不存在接收明文选项的字段。
func TestNoPlaintextOptionFieldInRequestPayloads(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	files := parseDir(t, fset, filepath.Join(root, "internal", "infrastructure", "http"))

	var violations []string
	for rel, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			decl, ok := node.(*ast.GenDecl)
			if !ok || decl.Tok != token.TYPE {
				return true
			}
			for _, spec := range decl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
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
					if _, hit := plaintextOptionJSONNames[strings.ToLower(name)]; !hit {
						continue
					}
					key := fmt.Sprintf("%s:%s.%s", rel, typeSpec.Name.Name, fieldName(field))
					if _, ok := plaintextOptionFieldAllowlist[key]; ok {
						continue
					}
					pos := fset.Position(field.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d —— 请求体结构体 %s 的字段 %s 收了明文选项",
						rel, pos.Line, typeSpec.Name.Name, fieldName(field),
					))
				}
			}
			return true
		})
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"服务端代码里不允许出现接收明文选项的字段（能力 A 的验收 ①）。"+
			"确实需要保留的，写进 plaintextOptionFieldAllowlist 并说明理由。")
}

// TestPlaintextOptionEntryPointsAreOnlyTheDebugChain 覆盖能力 A 的验收 ①
// 在用例层的部分：接受明文选项的入口只有 debug 链那几条。
func TestPlaintextOptionEntryPointsAreOnlyTheDebugChain(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	files := parseDir(t, fset, filepath.Join(root, "internal", "usecase"))

	var violations []string
	for rel, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			for _, param := range fn.Type.Params.List {
				if !isStringType(param.Type) {
					continue
				}
				for _, name := range param.Names {
					if name.Name != "option" && name.Name != "candidate" {
						continue
					}
					key := entryPointKey(rel, fn)
					if _, ok := plaintextOptionEntryPointAllowlist[key]; ok {
						continue
					}
					pos := fset.Position(fn.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d —— %s 接受明文选项参数 %q",
						rel, pos.Line, key, name.Name,
					))
				}
			}
		}
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"用例层不允许出现第二个「接受明文选项」的入口。"+
			"确实需要保留的，写进 plaintextOptionEntryPointAllowlist 并说明理由。")
}

// TestPlaintextOptionCallSitesAreOnlyTheDebugChain 断言 debug 链上的函数
// 没有被别处调用 —— 链子只通向 debug 门控的 handler。
func TestPlaintextOptionCallSitesAreOnlyTheDebugChain(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	files := parseDir(t, fset, filepath.Join(root, "internal"))

	var violations []string
	for rel, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			name := calledName(call)
			allowed, tracked := plaintextOptionCallSiteAllowlist[name]
			if !tracked {
				return true
			}
			if _, ok := allowed[rel]; ok {
				return true
			}
			pos := fset.Position(call.Pos())
			violations = append(violations, fmt.Sprintf(
				"%s:%d —— %s 在这里被调用（只允许 %s）",
				rel, pos.Line, name, strings.Join(sortedKeys(allowed), " / "),
			))
			return true
		})
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"debug 链上的函数被链子之外的地方调用了 —— 这条链只能通向 debug 门控的 handler。")
}

// TestPlaintextOptionPathsAreUnreachableInReleaseMode 让豁免表**自证**：
// 被豁免的那条路径在 release 模式下必须真的不可达。
//
// 这里刻意用「空依赖」的 handler：debug 模式下它会因 tallyUC 为 nil 走到
// 500（说明请求确实进了 handler），release 模式下必须是 404（说明它被门控
// 挡在门外、连 handler 都没进）。
func TestPlaintextOptionPathsAreUnreachableInReleaseMode(t *testing.T) {
	engine := releaseEngine(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/proposals/p-1/ballot-draft",
		strings.NewReader(`{"option":"candidate-A"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"release 模式下 ballot-draft 必须 404 —— 豁免表里那条路径在生产里得是死的")

	debugEngine := debugEngine(t)
	debugRec := httptest.NewRecorder()
	debugReq := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/proposals/p-1/ballot-draft",
		strings.NewReader(`{"option":"candidate-A"}`),
	)
	debugReq.Header.Set("Content-Type", "application/json")
	debugEngine.ServeHTTP(debugRec, debugReq)
	require.NotEqual(t, http.StatusNotFound, debugRec.Code,
		"debug 模式下这条路径必须是活的 —— 404 说明门控写错了，豁免表的前提不成立")
}

// releaseEngine 造一个 release 模式（debugMode=false）的引擎。
func releaseEngine(t *testing.T) *gin.Engine {
	t.Helper()
	handler := httpapi.NewHandlerWithTallyAndMode(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		false,
		zap.NewNop(),
	)
	return httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()
}

// debugEngine 造一个 debug 模式的引擎。
func debugEngine(t *testing.T) *gin.Engine {
	t.Helper()
	handler := httpapi.NewHandlerWithTallyAndMode(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		true,
		zap.NewNop(),
	)
	return httpapi.NewServer(&config.Config{
		ServerAddress:    ":0",
		GinMode:          gin.TestMode,
		AdminBearerToken: "secret",
	}, handler, zap.NewNop()).Engine()
}

// repoRoot 从本文件的位置往上找 go.mod，拿到仓库根。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "拿不到本文件路径")
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "找不到仓库根（go.mod）")
		dir = parent
	}
}

// parseDir 解析目录下的所有非测试 Go 文件，返回「相对仓库根的路径 → 文件」。
func parseDir(t *testing.T, fset *token.FileSet, dir string) map[string]*ast.File {
	t.Helper()
	root := repoRoot(t)
	out := map[string]*ast.File{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		out[filepath.ToSlash(rel)] = parsed
		return nil
	})
	require.NoError(t, err)
	return out
}

// jsonTagName 取字段的 json tag 名（没有 tag 或 `-` 时返回空串）。
func jsonTagName(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	value, ok := reflect.StructTag(raw).Lookup("json")
	if !ok {
		return ""
	}
	name := strings.Split(value, ",")[0]
	if name == "-" {
		return ""
	}
	return name
}

// fieldName 取字段名（匿名字段返回空串）。
func fieldName(field *ast.Field) string {
	if len(field.Names) == 0 {
		return ""
	}
	return field.Names[0].Name
}

// isStringType 判断参数类型是不是 string。
func isStringType(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "string"
}

// entryPointKey 生成入口豁免表的 key：文件路径 + 接收者类型 + 函数名。
func entryPointKey(rel string, fn *ast.FuncDecl) string {
	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		name = "(" + receiverTypeName(fn.Recv.List[0].Type) + ")." + name
	}
	return rel + ":" + name
}

// receiverTypeName 取接收者类型名（保留指针标记，去掉泛型参数）。
func receiverTypeName(expr ast.Expr) string {
	switch typed := expr.(type) {
	case *ast.StarExpr:
		return "*" + receiverTypeName(typed.X)
	case *ast.Ident:
		return typed.Name
	case *ast.IndexExpr:
		return receiverTypeName(typed.X)
	case *ast.IndexListExpr:
		return receiverTypeName(typed.X)
	default:
		return ""
	}
}

// calledName 取被调用函数的最后一个名字（方法调用看 Sel）。
func calledName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		return fun.Sel.Name
	default:
		return ""
	}
}

// sortedKeys 返回 map 的键（排序后）。
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
