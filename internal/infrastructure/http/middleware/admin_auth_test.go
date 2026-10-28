package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func newAdminEngine(expectedToken string) *gin.Engine {
	engine := gin.New()
	engine.Use(ErrorHandler(nil))
	engine.Use(RequireAdminBearerToken(expectedToken))
	engine.POST("/admin", func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	return engine
}

func TestRequireAdminBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing_token", func(t *testing.T) {
		engine := newAdminEngine("secret")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid_token", func(t *testing.T) {
		engine := newAdminEngine("secret")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer secret")
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("configured_token_is_trimmed", func(t *testing.T) {
		engine := newAdminEngine("  secret  ")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer secret")
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})

	t.Run("empty_configured_token_rejects_everything", func(t *testing.T) {
		engine := newAdminEngine("   ")

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer secret")
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

// 错误 token 与畸形 Authorization 头必须一律 401，且响应体不得泄露 token。
func TestRequireAdminBearerToken_RejectsBadCredentials(t *testing.T) {
	gin.SetMode(gin.TestMode)

	const expected = "super-secret-admin-token"

	cases := []struct {
		name   string
		header string
	}{
		{name: "wrong_token", header: "Bearer nope"},
		{name: "empty_bearer_value", header: "Bearer"},
		{name: "bearer_with_only_spaces", header: "Bearer    "},
		{name: "basic_scheme", header: "Basic super-secret-admin-token"},
		{name: "no_scheme", header: "super-secret-admin-token"},
		{name: "lowercase_scheme", header: "bearer super-secret-admin-token"},
		{name: "double_bearer", header: "Bearer Bearer super-secret-admin-token"},
		{name: "token_prefix", header: "Bearer super-secret-admin-toke"},
		{name: "token_extra_suffix", header: "Bearer super-secret-admin-tokenx"},
		{name: "shorter_token", header: "Bearer super"},
		{name: "longer_token", header: "Bearer super-secret-admin-token-and-more"},
		{name: "empty_header", header: ""},
		{name: "non_ascii_token", header: "Bearer super-sëcret-admin-token"},
		{name: "token_with_newline", header: "Bearer super-secret-admin-token\r\nX-Injected: 1"},
		{name: "token_with_trailing_crlf", header: "Bearer super-secret-admin-token\r\n"},
		{name: "token_with_embedded_null", header: "Bearer super-secret-admin-token\x00"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			engine := newAdminEngine(expected)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/admin", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			engine.ServeHTTP(rec, req)

			require.Equal(t, http.StatusUnauthorized, rec.Code)
			body := rec.Body.String()
			require.NotContains(t, body, expected)
			require.False(t, strings.Contains(body, "super-secret"), "响应体泄露了 token 前缀")
		})
	}
}

// 多余空格沿用既有宽松行为：Bearer 后的空白被裁掉后仍应匹配。
func TestRequireAdminBearerToken_AllowsPaddingAfterScheme(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := newAdminEngine("secret")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/admin", nil)
	req.Header.Set("Authorization", "Bearer   secret")
	engine.ServeHTTP(rec, req)
	require.Equal(t, http.StatusNoContent, rec.Code)
}

// TestRequireAdminRole_Containment 钉住角色包含关系：
// admin ⊃ operator ⊃ auditor —— 上一级令牌能过下一级的门，反之不行。
func TestRequireAdminRole_Containment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokens := AdminTokens{Admin: "t-admin", Operator: "t-operator", Auditor: "t-auditor"}

	cases := []struct {
		required string
		token    string
		allowed  bool
	}{
		{RoleAuditor, "t-auditor", true},
		{RoleAuditor, "t-operator", true},
		{RoleAuditor, "t-admin", true},
		{RoleOperator, "t-auditor", false},
		{RoleOperator, "t-operator", true},
		{RoleOperator, "t-admin", true},
		{RoleAdmin, "t-auditor", false},
		{RoleAdmin, "t-operator", false},
		{RoleAdmin, "t-admin", true},
	}
	for _, tc := range cases {
		t.Run(tc.required+"/"+tc.token, func(t *testing.T) {
			engine := gin.New()
			engine.Use(ErrorHandler(nil))
			engine.Use(RequireAdminRole(tokens, tc.required))
			engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			engine.ServeHTTP(rec, req)

			if tc.allowed {
				require.Equal(t, http.StatusNoContent, rec.Code)
			} else {
				require.Equal(t, http.StatusUnauthorized, rec.Code,
					"权限不足与令牌错误同形地返回 401（403 会泄露路由的权限档位）")
			}
		})
	}
}

func TestRequireAdminRole_RejectsBadHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tokens := AdminTokens{Admin: "t-admin", Operator: "t-operator", Auditor: "t-auditor"}
	engine := gin.New()
	engine.Use(ErrorHandler(nil))
	engine.Use(RequireAdminRole(tokens, RoleAuditor))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	// 与单一令牌时代同一套恶意头：三个角色上一律拒绝。
	for _, header := range []string{
		"Bearer t-auditor\r\n",
		"Bearer t-auditor\x00",
		"Bearer ",
		"Basic dG9rZW4=",
		"",
	} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		if header != "" {
			req.Header.Set("Authorization", header)
		}
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "header %q 应当被拒", header)
	}
}

// TestRequireAdminRole_NoCandidateConfigured 一个候选令牌都没有 → 一律拒绝
// （中间件自己就是最后一道闸，不依赖配置层校验）。
func TestRequireAdminRole_NoCandidateConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)

	engine := gin.New()
	engine.Use(ErrorHandler(nil))
	engine.Use(RequireAdminRole(AdminTokens{Auditor: "t-auditor"}, RoleOperator))
	engine.GET("/x", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	for _, token := range []string{"t-auditor", "anything"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}
}
