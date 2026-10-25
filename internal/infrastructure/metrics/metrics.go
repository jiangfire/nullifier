// Package metrics 提供服务的 Prometheus 指标。
//
// ⚠️ **只暴露计数与耗时** —— 这里没有任何选票内容、候选人票数分布、nullifier、
// user_id 或凭证标识。指标是运维视角的「发生了多少次、多久」，不是投票内容
// 的第二个出口。往这里加任何带票面信息的标签都是安全事故。
//
// 指标实例由依赖注入创建并传递（见 internal/di/providers.go）：handler 是业务
// 事件的发生地，资格校验适配器是资格证明真正被验证的地方，两者把计数写进
// 同一个实例，server 再把它暴露成 GET /metrics。
package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// namespace 是所有指标的前缀。
const namespace = "anonvote"

// 业务事件的结果标签取值。
const (
	// ResultAccepted 表示操作成功。
	ResultAccepted = "accepted"
	// ResultRejected 表示操作被拒。
	ResultRejected = "rejected"
	// ResultValid 表示资格证明通过了校验。
	ResultValid = "valid"
	// ResultInvalid 表示资格证明没通过校验。
	ResultInvalid = "invalid"
)

// Metrics 持有服务的全部指标。
//
// 所有记录方法都容忍 nil 接收者：没有配置指标时（单元测试、嵌入式用法）
// 调用它们是安全的空操作，而不是 panic。
type Metrics struct {
	registry *prometheus.Registry

	httpRequests *prometheus.CounterVec
	httpDuration *prometheus.HistogramVec

	votesAccepted prometheus.Counter
	votesRejected *prometheus.CounterVec

	credentialClaims     *prometheus.CounterVec
	credentialBlindSigns *prometheus.CounterVec
	credentialAttests    *prometheus.CounterVec

	eligibilityChecks *prometheus.CounterVec

	rateLimited *prometheus.CounterVec

	tallyResultsPublished prometheus.Counter
}

// New 造一个带独立注册表的指标实例。
//
// 用独立注册表而不是全局默认的：测试里每个实例互不干扰，
// 指标名也不会因为重复注册而 panic。
func New() *Metrics {
	m := &Metrics{
		registry: prometheus.NewRegistry(),
	}

	m.httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "http_requests_total",
		Help:      "HTTP 请求总数，按方法、路由与状态码统计。",
	}, []string{"method", "route", "status"})

	m.httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: namespace,
		Name:      "http_request_duration_seconds",
		Help:      "HTTP 请求耗时分布，按方法与路由统计。",
		Buckets:   prometheus.DefBuckets,
	}, []string{"method", "route"})

	m.votesAccepted = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "votes_accepted_total",
		Help:      "成功入库的选票数。",
	})

	m.votesRejected = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "votes_rejected_total",
		Help:      "被拒的投票提交数，按错误码统计。",
	}, []string{"code"})

	m.credentialClaims = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "credential_claims_total",
		Help:      "凭证领取请求数，按结果统计。",
	}, []string{"result"})

	m.credentialBlindSigns = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "credential_blind_signs_total",
		Help:      "盲签名请求数，按结果统计。",
	}, []string{"result"})

	m.credentialAttests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "credential_attests_total",
		Help:      "凭证 attest 请求数，按结果统计。",
	}, []string{"result"})

	m.eligibilityChecks = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "eligibility_checks_total",
		Help:      "资格证明校验数，按校验结果统计。",
	}, []string{"result"})

	m.tallyResultsPublished = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "tally_results_published_total",
		Help:      "发布出去的计票结果数。",
	})

	// 限流计数。route 用**路由模板**（/api/v1/proposals/:proposal_id/votes），
	// 不是原始路径 —— 否则每个提案 ID 都会变成一个标签值，标签集会爆。
	m.rateLimited = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "rate_limited_total",
		Help:      "被限流挡下的请求数，按路由统计。",
	}, []string{"route"})

	m.registry.MustRegister(
		m.httpRequests,
		m.httpDuration,
		m.votesAccepted,
		m.votesRejected,
		m.credentialClaims,
		m.credentialBlindSigns,
		m.credentialAttests,
		m.eligibilityChecks,
		m.rateLimited,
		m.tallyResultsPublished,
	)

	return m
}

// Registry 返回指标注册表。
func (m *Metrics) Registry() *prometheus.Registry {
	if m == nil {
		return nil
	}
	return m.registry
}

// Handler 返回暴露 Prometheus 文本格式的 HTTP 处理器。
func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
}

// ObserveHTTPRequest 记录一次 HTTP 请求。
func (m *Metrics) ObserveHTTPRequest(route string, method string, status int, elapsed time.Duration) {
	if m == nil {
		return
	}
	m.httpRequests.WithLabelValues(method, route, statusLabel(status)).Inc()
	m.httpDuration.WithLabelValues(method, route).Observe(elapsed.Seconds())
}

// RecordVoteAccepted 记录一张成功入库的选票。
func (m *Metrics) RecordVoteAccepted() {
	if m == nil {
		return
	}
	m.votesAccepted.Inc()
}

// RecordVoteRejected 记录一次被拒的投票提交。code 是机器可读的错误码。
func (m *Metrics) RecordVoteRejected(code string) {
	if m == nil {
		return
	}
	m.votesRejected.WithLabelValues(normalizeCode(code)).Inc()
}

// RecordCredentialClaim 记录一次凭证领取请求。
func (m *Metrics) RecordCredentialClaim(result string) {
	if m == nil {
		return
	}
	m.credentialClaims.WithLabelValues(result).Inc()
}

// RecordCredentialBlindSign 记录一次盲签名请求。
func (m *Metrics) RecordCredentialBlindSign(result string) {
	if m == nil {
		return
	}
	m.credentialBlindSigns.WithLabelValues(result).Inc()
}

// RecordCredentialAttest 记录一次凭证 attest 请求。
func (m *Metrics) RecordCredentialAttest(result string) {
	if m == nil {
		return
	}
	m.credentialAttests.WithLabelValues(result).Inc()
}

// RecordEligibilityCheck 记录一次资格证明校验的结果。
func (m *Metrics) RecordEligibilityCheck(result string) {
	if m == nil {
		return
	}
	m.eligibilityChecks.WithLabelValues(result).Inc()
}

// RecordTallyResultPublished 记录一次计票结果发布。
func (m *Metrics) RecordTallyResultPublished() {
	if m == nil {
		return
	}
	m.tallyResultsPublished.Inc()
}

// RecordRateLimited 记录一次被限流挡下的请求。
//
// route 必须是**路由模板**而不是原始路径：每个提案 ID 都变成一个标签值的话，
// 标签集会爆，Prometheus 侧也会变成一个查询不动的东西。
func (m *Metrics) RecordRateLimited(route string) {
	if m == nil {
		return
	}
	m.rateLimited.WithLabelValues(normalizeRoute(route)).Inc()
}

// normalizeRoute 保证 route 标签永远非空 —— 空标签会让指标多出一组无名序列。
func normalizeRoute(route string) string {
	if route == "" {
		return "unmatched"
	}
	return route
}

// statusLabel 把状态码转成标签值。
func statusLabel(status int) string {
	return strconv.Itoa(status)
}

// normalizeCode 保证错误码标签永远非空 —— 空标签会让指标多出一组无名序列，
// 反而更难读。
func normalizeCode(code string) string {
	if code == "" {
		return "UNKNOWN"
	}
	return code
}
