package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestRequireAdminBearerToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("missing_token", func(t *testing.T) {
		engine := gin.New()
		engine.Use(ErrorHandler(nil))
		engine.Use(RequireAdminBearerToken("secret"))
		engine.POST("/admin", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("valid_token", func(t *testing.T) {
		engine := gin.New()
		engine.Use(ErrorHandler(nil))
		engine.Use(RequireAdminBearerToken("secret"))
		engine.POST("/admin", func(c *gin.Context) {
			c.Status(http.StatusNoContent)
		})

		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/admin", nil)
		req.Header.Set("Authorization", "Bearer secret")
		engine.ServeHTTP(rec, req)
		require.Equal(t, http.StatusNoContent, rec.Code)
	})
}
