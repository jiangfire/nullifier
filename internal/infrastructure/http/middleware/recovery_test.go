package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRecovery_ConvertsPanicToInternalError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, tc := range []struct {
		name   string
		logger *zap.Logger
	}{
		{name: "nil_logger", logger: nil},
		{name: "explicit_logger", logger: zap.NewNop()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(Recovery(tc.logger))
			engine.GET("/panic", func(c *gin.Context) {
				panic("boom")
			})

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/panic", nil)
			engine.ServeHTTP(rec, req)

			require.Equal(t, http.StatusInternalServerError, rec.Code)
			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			require.Equal(t, "INTERNAL_ERROR", body["code"])
			require.Equal(t, "internal server error", body["error"])
		})
	}
}
