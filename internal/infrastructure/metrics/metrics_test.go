package metrics

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// populateAll 把每个指标都写一笔。
//
// Gather 只返回**有样本**的指标族 —— 没被写过的 CounterVec 根本不出现在
// 采集结果里，所以要先把它们都填上再断言指标名。
func populateAll(m *Metrics) {
	m.ObserveHTTPRequest("/api/v1/proposals", "GET", 200, time.Millisecond)
	m.RecordVoteAccepted()
	m.RecordVoteRejected("DOUBLE_VOTING")
	m.RecordCredentialClaim(ResultAccepted)
	m.RecordCredentialBlindSign(ResultAccepted)
	m.RecordCredentialAttest(ResultAccepted)
	m.RecordEligibilityCheck(ResultValid)
	m.RecordTallyResultPublished()
}

func TestNew_RegistersExpectedMetrics(t *testing.T) {
	t.Parallel()

	m := New()
	require.NotNil(t, m.Registry())
	populateAll(m)

	families, err := m.Registry().Gather()
	require.NoError(t, err)

	names := make([]string, 0, len(families))
	for _, family := range families {
		names = append(names, family.GetName())
	}
	// 指标名是运维契约的一部分（告警规则按名字写），所以钉死它。
	assert.ElementsMatch(t, []string{
		"anonvote_http_request_duration_seconds",
		"anonvote_http_requests_total",
		"anonvote_credential_attests_total",
		"anonvote_credential_blind_signs_total",
		"anonvote_credential_claims_total",
		"anonvote_eligibility_checks_total",
		"anonvote_tally_results_published_total",
		"anonvote_votes_accepted_total",
		"anonvote_votes_rejected_total",
	}, names)
}

// TestExposedSurface_HasNoVoteContent 守卫「指标端点只暴露计数与耗时」。
//
// 指标里一旦出现带票面信息的标签（nullifier、user_id、候选人票数……），
// 它就变成了投票内容的第二个出口 —— 那是要出安全事故的。这里把允许出现的
// 标签名逐个钉死：多一个标签就是多一条泄漏路径。
func TestExposedSurface_HasNoVoteContent(t *testing.T) {
	t.Parallel()

	m := New()

	m.ObserveHTTPRequest("/api/v1/proposals/:proposal_id/votes", "POST", 201, 12*time.Millisecond)
	m.RecordVoteAccepted()
	m.RecordVoteRejected("DOUBLE_VOTING")
	m.RecordCredentialClaim(ResultAccepted)
	m.RecordCredentialBlindSign(ResultRejected)
	m.RecordCredentialAttest(ResultAccepted)
	m.RecordEligibilityCheck(ResultInvalid)
	m.RecordTallyResultPublished()
	m.RecordRateLimited("/api/v1/proposals/:proposal_id/votes")

	allowedLabels := map[string]bool{"method": true,
		"route":  true,
		"status": true,
		"result": true,
		"code":   true,
	}

	families, err := m.Registry().Gather()
	require.NoError(t, err)
	require.NotEmpty(t, families)

	for _, family := range families {
		for _, metric := range family.GetMetric() {
			for _, label := range metric.GetLabel() {
				assert.True(t, allowedLabels[label.GetName()],
					"指标 %s 出现了未授权的标签 %s —— 指标端点只允许暴露计数与耗时",
					family.GetName(), label.GetName(),
				)
			}
		}
	}
}

func TestRecordVoteCounters(t *testing.T) {
	t.Parallel()

	m := New()

	m.RecordVoteAccepted()
	m.RecordVoteAccepted()
	m.RecordVoteRejected("DOUBLE_VOTING")

	assert.Equal(t, 2.0, testutil.ToFloat64(m.votesAccepted))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.votesRejected.WithLabelValues("DOUBLE_VOTING")))
}

// TestRecordVoteRejected_EmptyCode 用 UNKNOWN 占位。
//
// 空标签会多出一组无名序列，指标更难读 —— 而且不同错误来源会混在一起。
func TestRecordVoteRejected_EmptyCode(t *testing.T) {
	t.Parallel()

	m := New()
	m.RecordVoteRejected("")

	assert.Equal(t, 1.0, testutil.ToFloat64(m.votesRejected.WithLabelValues("UNKNOWN")))
}

func TestRecordCredentialAndEligibilityCounters(t *testing.T) {
	t.Parallel()

	m := New()

	m.RecordCredentialClaim(ResultAccepted)
	m.RecordCredentialClaim(ResultRejected)
	m.RecordCredentialBlindSign(ResultAccepted)
	m.RecordCredentialAttest(ResultRejected)
	m.RecordEligibilityCheck(ResultValid)
	m.RecordEligibilityCheck(ResultInvalid)
	m.RecordTallyResultPublished()

	assert.Equal(t, 1.0, testutil.ToFloat64(m.credentialClaims.WithLabelValues(ResultAccepted)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.credentialClaims.WithLabelValues(ResultRejected)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.credentialBlindSigns.WithLabelValues(ResultAccepted)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.credentialAttests.WithLabelValues(ResultRejected)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.eligibilityChecks.WithLabelValues(ResultValid)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.eligibilityChecks.WithLabelValues(ResultInvalid)))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.tallyResultsPublished))
}

// TestRecordRateLimited 验证限流计数按路由分桶。
func TestRecordRateLimited(t *testing.T) {
	t.Parallel()

	m := New()

	m.RecordRateLimited("/api/v1/proposals/:proposal_id/votes")
	m.RecordRateLimited("/api/v1/proposals/:proposal_id/votes")
	m.RecordRateLimited("/api/v1/proposals")

	assert.Equal(t, 2.0, testutil.ToFloat64(m.rateLimited.WithLabelValues("/api/v1/proposals/:proposal_id/votes")))
	assert.Equal(t, 1.0, testutil.ToFloat64(m.rateLimited.WithLabelValues("/api/v1/proposals")))
}

// TestRecordRateLimited_NormalizesEmptyRoute 钉住空路由标签被归一化 ——
// 空标签会让指标多出一组无名序列，反而更难读。
func TestRecordRateLimited_NormalizesEmptyRoute(t *testing.T) {
	t.Parallel()

	m := New()

	m.RecordRateLimited("")

	assert.Equal(t, 1.0, testutil.ToFloat64(m.rateLimited.WithLabelValues("unmatched")))
}

func TestObserveHTTPRequest(t *testing.T) {
	t.Parallel()

	m := New()

	m.ObserveHTTPRequest("/api/v1/proposals", "GET", 200, 3*time.Millisecond)
	m.ObserveHTTPRequest("/api/v1/proposals", "GET", 200, 5*time.Millisecond)

	assert.Equal(t, 2.0, testutil.ToFloat64(m.httpRequests.WithLabelValues("GET", "/api/v1/proposals", "200")))

	families, err := m.Registry().Gather()
	require.NoError(t, err)

	var histogram *dto.Histogram
	for _, family := range families {
		if family.GetName() == "anonvote_http_request_duration_seconds" {
			histogram = family.GetMetric()[0].GetHistogram()
		}
	}
	require.NotNil(t, histogram)
	assert.Equal(t, uint64(2), histogram.GetSampleCount())
}

// TestNilMetrics_AreNoOps 覆盖「没有配置指标时调用是安全的空操作」。
func TestNilMetrics_AreNoOps(t *testing.T) {
	t.Parallel()

	var m *Metrics

	assert.NotPanics(t, func() {
		m.ObserveHTTPRequest("/x", "GET", 200, time.Second)
		m.RecordVoteAccepted()
		m.RecordVoteRejected("X")
		m.RecordCredentialClaim(ResultAccepted)
		m.RecordCredentialBlindSign(ResultAccepted)
		m.RecordCredentialAttest(ResultAccepted)
		m.RecordEligibilityCheck(ResultValid)
		m.RecordTallyResultPublished()
	})
	assert.Nil(t, m.Registry())
}

func TestHandler_ServesPrometheusText(t *testing.T) {
	t.Parallel()

	m := New()
	m.RecordVoteAccepted()

	rec := httptest.NewRecorder()
	m.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))

	require.Equal(t, 200, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "anonvote_votes_accepted_total 1")
}
