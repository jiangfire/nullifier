package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"anonvote/internal/infrastructure/metrics"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// counterValue 返回某个指标族的计数总和（按标签拆开时求和）。
func counterValue(t *testing.T, m *metrics.Metrics, name string) float64 {
	t.Helper()
	families, err := m.Registry().Gather()
	require.NoError(t, err)

	total := 0.0
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			total += metric.GetCounter().GetValue()
		}
	}
	return total
}

// counterLabels 返回某个指标族出现过的标签组合。
func counterLabels(t *testing.T, m *metrics.Metrics, name string) [][]string {
	t.Helper()
	families, err := m.Registry().Gather()
	require.NoError(t, err)

	out := make([][]string, 0)
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			pair := make([]string, 0, len(metric.GetLabel()))
			for _, label := range metric.GetLabel() {
				pair = append(pair, label.GetName()+"="+label.GetValue())
			}
			out = append(out, pair)
		}
	}
	return out
}

func newMetricsEngine(m *metrics.Metrics) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(MetricsMiddleware(m))
	return engine
}

func TestMetricsMiddleware_RecordsRouteAndStatus(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	engine := newMetricsEngine(m)
	engine.GET("/api/v1/proposals", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/proposals", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 1.0, counterValue(t, m, "anonvote_http_requests_total"))
	assert.Contains(t, counterLabels(t, m, "anonvote_http_requests_total"),
		[]string{"method=GET", "route=/api/v1/proposals", "status=200"})
}

func TestMetricsMiddleware_UnmatchedRouteUsesPlaceholder(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	engine := newMetricsEngine(m)
	engine.GET("/api/v1/proposals", func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil))
	require.Equal(t, http.StatusNotFound, rec.Code)

	// 匹配不到路由时不能拿原始路径当标签 —— 路径里带着提案 ID，
	// 拿它当标签值会让指标基数爆炸。
	assert.Contains(t, counterLabels(t, m, "anonvote_http_requests_total"),
		[]string{"method=GET", "route=unmatched", "status=404"})
}

func TestMetricsMiddleware_SkipsMetricsEndpointItself(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	engine := newMetricsEngine(m)
	engine.GET(MetricsEndpointPath, func(c *gin.Context) { c.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, MetricsEndpointPath, nil))
	require.Equal(t, http.StatusOK, rec.Code)

	// 采集自身的请求不计数 —— 否则请求数会被采集流量本身淹没。
	assert.Equal(t, 0.0, counterValue(t, m, "anonvote_http_requests_total"))
}

func TestMetricsMiddleware_RecordsErrorStatus(t *testing.T) {
	t.Parallel()

	m := metrics.New()
	engine := newMetricsEngine(m)
	engine.GET("/boom", func(c *gin.Context) { c.String(http.StatusTeapot, "boom") })

	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	require.Equal(t, http.StatusTeapot, rec.Code)

	assert.Contains(t, counterLabels(t, m, "anonvote_http_requests_total"),
		[]string{"method=GET", "route=/boom", "status=418"})
}
