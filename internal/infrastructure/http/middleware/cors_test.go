package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestCORS_AllowsOriginOnSimpleRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CORS())
	engine.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "GET,POST,OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Content-Type,Authorization", rec.Header().Get("Access-Control-Allow-Headers"))
}

func TestCORS_HandlesPreflightRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(CORS())
	engine.POST("/api/v1/votes", func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/votes", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "GET,POST,OPTIONS", rec.Header().Get("Access-Control-Allow-Methods"))
}
