package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
)

const bearerScheme = "Bearer "

// 管理面的三个角色。包含关系是刻意的：admin 能做 operator 的一切，
// operator 能做 auditor 的一切 —— 上一级令牌覆盖下一级的全部面。
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleAuditor  = "auditor"
)

// AdminTokens 是管理面的分级令牌。
//
// 旧的单值 admin_bearer_token 等价于 Admin（存量部署零改动升级）。
type AdminTokens struct {
	Admin    string
	Operator string
	Auditor  string
}

// RequireAdminBearerToken 是旧入口：单一令牌、admin 一档。
//
// 存量调用与测试都从这里走；分级部署用 RequireAdminRole。
func RequireAdminBearerToken(token string) gin.HandlerFunc {
	return RequireAdminRole(AdminTokens{Admin: token}, RoleAdmin)
}

// RequireAdminRole 返回按角色分级的鉴权中间件：请求令牌必须**至少**具备
// required 的角色（admin > operator > auditor）。
//
// 权限不足与令牌错误**同形**地返回 401 —— 回 403 会告诉持低级令牌的人
// 「这个路由存在，且需要更高权限」；401 什么都不泄露。所有候选令牌都做
// 常数时间比较。
func RequireAdminRole(tokens AdminTokens, required string) gin.HandlerFunc {
	// 候选集：持有 required 或更高角色的全部令牌。
	candidates := make([]string, 0, 3)
	add := func(token, role string) {
		if roleAtLeast(role, required) {
			candidates = append(candidates, strings.TrimSpace(token))
		}
	}
	add(tokens.Admin, RoleAdmin)
	add(tokens.Operator, RoleOperator)
	add(tokens.Auditor, RoleAuditor)

	return func(c *gin.Context) {
		if len(candidates) == 0 {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		provided, ok := extractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}
		for _, expected := range candidates {
			if expected != "" && constantTimeEqual(provided, expected) {
				c.Next()
				return
			}
		}
		c.Error(usecase.ErrUnauthorized)
		c.Abort()
	}
}

// extractBearerToken 从 Authorization 头取出令牌。
//
// 控制字符一律拒绝。合法 Authorization 头不会含 CR/LF/NUL，出现即说明有人
// 尝试注入；而且 TrimSpace 会把尾随的 CRLF 悄悄裁掉，让 "Bearer <token>\r\n"
// 看起来是一个合法头。
func extractBearerToken(header string) (string, bool) {
	if strings.ContainsAny(header, "\r\n\x00") {
		return "", false
	}
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(header, bearerScheme) {
		return "", false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(header, bearerScheme))
	if provided == "" {
		return "", false
	}
	return provided, true
}

// roleAtLeast 报告某个角色是否至少达到 required。
func roleAtLeast(holder, required string) bool {
	switch required {
	case RoleAuditor:
		return holder == RoleAuditor || holder == RoleOperator || holder == RoleAdmin
	case RoleOperator:
		return holder == RoleOperator || holder == RoleAdmin
	case RoleAdmin:
		return holder == RoleAdmin
	default:
		return false
	}
}

// constantTimeEqual 比较两个令牌，耗时与内容无关。
//
// 先各自做 SHA-256 摘要再逐字节比较：摘要恒为 32 字节，
// 因此长度不同的令牌也不会让 ConstantTimeCompare 因长度不等而提前返回 ——
// 否则「长度」本身就成了可爆破的侧信道。
func constantTimeEqual(a, b string) bool {
	sumA := sha256.Sum256([]byte(a))
	sumB := sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(sumA[:], sumB[:]) == 1
}
