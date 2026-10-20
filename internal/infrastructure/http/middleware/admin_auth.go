package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"strings"

	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
)

const bearerScheme = "Bearer "

func RequireAdminBearerToken(token string) gin.HandlerFunc {
	expected := strings.TrimSpace(token)

	return func(c *gin.Context) {
		if expected == "" {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		authHeader := c.GetHeader("Authorization")
		// 控制字符一律拒绝。合法 Authorization 头不会含 CR/LF/NUL，
		// 出现即说明有人尝试注入；而且 TrimSpace 会把尾随的 CRLF 悄悄
		// 裁掉，让 "Bearer <token>\r\n" 看起来是一个合法头。
		if strings.ContainsAny(authHeader, "\r\n\x00") {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		authHeader = strings.TrimSpace(authHeader)
		if !strings.HasPrefix(authHeader, bearerScheme) {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}
		provided := strings.TrimSpace(strings.TrimPrefix(authHeader, bearerScheme))
		if !constantTimeEqual(provided, expected) {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Next()
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
