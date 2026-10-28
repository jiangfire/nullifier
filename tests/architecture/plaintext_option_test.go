// Package architecture 放的是「架构不变量」的守卫测试。
//
// 它们不看业务结果，只看**代码里还剩不该剩的东西**。
//
// 背景：能力 A（选票内容对服务端不可知）的底线是「服务端不得拿到明文选项」。
// 这条底线以前靠人工 grep —— 人会忘，也会在改动后不再复查。这里把它变成
// 持续可执行的门禁：
//
//  1. HTTP 层的请求体结构里不允许出现明文选项字段（json tag）；
//  2. usecase 层不允许出现「接受明文选项」的入口；
//  3. 服务端代加密的那条链（ballot-draft）不许复活；
//  4. ballot-draft 端点不许复活（路由没注册 → 404）。
//
// **本文件不设豁免机制。** 「服务端知道选了哪个」没有例外：真要加，得先想清楚
// 是不是该改的是客户端。真要加豁免，就得同时改这个文件 —— 让它在 diff 里显眼。
//
// 扫描范围说明：只扫 `internal/`（服务端）。`cmd/` 里也有明文选项字段
// （`cmd/cli` 的 ballot-encrypt 命令、`cmd/ballot-wasm` 的 prove 请求），
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

// removedServerSideOptionSymbols 是「服务端代加密」那条链上的符号。
//
// 它们在 2026-10-25 被删掉（ballot-draft 端点 + usecase 的 EncryptVoteOption
// / PrepareBallotDraft / BallotDraft / proposalOptionIndex / proposalAllowsOption）。
// 这里钉住它们的消失 —— 有人不小心把「服务端替用户加密」加回来时，这条会红。
var removedServerSideOptionSymbols = map[string]struct{}{
	"PrepareBallotDraft":     {},
	"EncryptVoteOption":      {},
	"BallotDraft":            {},
	"proposalOptionIndex":    {},
	"proposalAllowsOption":   {},
	"developmentProofBase64": {},
}

// TestNoPlaintextOptionFieldInRequestPayloads 覆盖能力 A 的底线：
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
		"服务端代码里不允许出现接收明文选项的字段（能力 A 的底线）。"+
			"选票密文必须由客户端生成，服务端只提供公开的计票公钥。")
}

// TestNoPlaintextOptionEntryPoints 覆盖能力 A 的底线在用例层的部分：
// 不允许出现「接受明文选项」的入口。
func TestNoPlaintextOptionEntryPoints(t *testing.T) {
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
					pos := fset.Position(fn.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d —— %s 接受明文选项参数 %q",
						rel, pos.Line, entryPointName(fn), name.Name,
					))
				}
			}
		}
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"用例层不允许出现「接受明文选项」的入口 —— "+
			"选票的加密与证明都在客户端完成。")
}

// TestServerSideOptionEncryptionChainIsGone 钉住「服务端代加密」那条链的消失。
func TestServerSideOptionEncryptionChainIsGone(t *testing.T) {
	root := repoRoot(t)
	fset := token.NewFileSet()
	files := parseDir(t, fset, filepath.Join(root, "internal"))

	var violations []string
	for rel, file := range files {
		for _, decl := range file.Decls {
			switch typed := decl.(type) {
			case *ast.FuncDecl:
				if _, tracked := removedServerSideOptionSymbols[typed.Name.Name]; tracked {
					pos := fset.Position(typed.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d —— %s 又出现了（服务端不该替用户加密选票）",
						rel, pos.Line, typed.Name.Name,
					))
				}
			case *ast.GenDecl:
				if typed.Tok != token.TYPE {
					continue
				}
				for _, spec := range typed.Specs {
					typeSpec, ok := spec.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if _, tracked := removedServerSideOptionSymbols[typeSpec.Name.Name]; !tracked {
						continue
					}
					pos := fset.Position(typeSpec.Pos())
					violations = append(violations, fmt.Sprintf(
						"%s:%d —— 类型 %s 又出现了（服务端不该替用户准备选票草稿）",
						rel, pos.Line, typeSpec.Name.Name,
					))
				}
			}
		}
	}

	sort.Strings(violations)
	require.Empty(t, violations,
		"服务端代加密的那条链（ballot-draft）已经删掉，不许复活。")
}

// TestBallotDraftEndpointIsNotRegistered 断言 ballot-draft 端点没有被注册：
// 无论什么模式，POST 它都是 404（gin 对未注册的路由返回 404）。
//
// 这条同时说明「为什么用 404 而不是 403」的老问题已经不存在了 —— 端点本身
// 就不在了。
func TestBallotDraftEndpointIsNotRegistered(t *testing.T) {
	engine := testEngine(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/proposals/p-1/ballot-draft",
		strings.NewReader(`{"option":"candidate-A"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNotFound, rec.Code,
		"ballot-draft 端点已被删除 —— 服务端不允许有接收明文选项的入口")
}

// testEngine 造一个最小引擎（依赖全 nil，只看路由是否存在）。
func testEngine(t *testing.T) *gin.Engine {
	t.Helper()
	handler := httpapi.NewHandlerWithTally(
		usecase.NewVotingUseCase(nil, nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
		usecase.NewRegisterUseCase(nil, nil, nil, zap.NewNop()),
		usecase.NewProposalUseCase(nil, nil, zap.NewNop()),
		usecase.NewParticipationUseCase(nil, nil, zap.NewNop()),
		usecase.NewTallyUseCase(nil, nil, nil, nil, nil, nil, nil, zap.NewNop()),
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

// entryPointName 生成入口的名字（方法带接收者类型）。
func entryPointName(fn *ast.FuncDecl) string {
	name := fn.Name.Name
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		name = "(" + receiverTypeName(fn.Recv.List[0].Type) + ")." + name
	}
	return name
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
