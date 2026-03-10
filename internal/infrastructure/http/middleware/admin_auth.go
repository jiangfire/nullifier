package middleware

import (
	"strings"

	"anonvote/internal/usecase"

	"github.com/gin-gonic/gin"
)

func RequireAdminBearerToken(token string) gin.HandlerFunc {
	expected := strings.TrimSpace(token)

	return func(c *gin.Context) {
		if expected == "" {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}
		if strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer ")) != expected {
			c.Error(usecase.ErrUnauthorized)
			c.Abort()
			return
		}

		c.Next()
	}
}
