package ratelimit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

func TestLimiter_AllowsUpToBurst(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 3})

	for i := 0; i < 3; i++ {
		allowed, _ := l.Allow("client-a")
		assert.True(t, allowed, "第 %d 次请求应当在突发额度内", i+1)
	}

	allowed, retryAfter := l.Allow("client-a")
	assert.False(t, allowed, "超出突发额度后应当被拒")
	assert.Greater(t, retryAfter, time.Duration(0), "被拒时应当给出重试等待时长")
}

func TestLimiter_DifferentKeysAreIndependent(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 2})

	require.True(t, mustAllow(l, "client-a"))
	require.True(t, mustAllow(l, "client-a"))
	require.False(t, mustAllow(l, "client-a"))

	// 另一个来源的额度不受影响 —— 这条是「不同 IP 互不影响」那条验收的根。
	assert.True(t, mustAllow(l, "client-b"))
	assert.True(t, mustAllow(l, "client-b"))
	assert.False(t, mustAllow(l, "client-b"))
}

func TestLimiter_TokensRefillOverTime(t *testing.T) {
	t.Parallel()

	// 每秒 20 个令牌 → 一个令牌 50ms。
	l := New(Config{Limit: rate.Limit(20), Burst: 1})

	require.True(t, mustAllow(l, "client-a"))
	require.False(t, mustAllow(l, "client-a"))

	time.Sleep(80 * time.Millisecond)

	assert.True(t, mustAllow(l, "client-a"), "过了补充间隔后应当恢复放行")
}

func TestLimiter_KeyIsNotStoredInPlainText(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 1})
	const secretIP = "203.0.113.7"

	mustAllow(l, secretIP)

	l.mu.Lock()
	defer l.mu.Unlock()
	for digest := range l.buckets {
		assert.NotContains(t, digest, secretIP, "桶的键里不应当出现明文来源")
	}
}

func TestLimiter_SameKeyHashesConsistently(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 1})
	mustAllow(l, "client-a")

	l.mu.Lock()
	defer l.mu.Unlock()
	require.Len(t, l.buckets, 1)
	for digest := range l.buckets {
		assert.Equal(t, l.digest("client-a"), digest)
	}
}

func TestLimiter_DifferentLimitersUseDifferentSalts(t *testing.T) {
	t.Parallel()

	// 盐每进程随机 → 两个限流器（不同路由组）之间的键不可关联。
	a := New(Config{Limit: rate.Limit(1), Burst: 1})
	b := New(Config{Limit: rate.Limit(1), Burst: 1})
	assert.NotEqual(t, a.digest("client-a"), b.digest("client-a"))
}

func TestLimiter_SweepRemovesIdleBuckets(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 1, IdleTTL: 30 * time.Millisecond})
	mustAllow(l, "client-a")
	mustAllow(l, "client-b")
	require.Equal(t, 2, l.Len())

	time.Sleep(60 * time.Millisecond)

	assert.Equal(t, 2, l.Sweep(), "空闲超过 TTL 的桶应当被清掉")
	assert.Equal(t, 0, l.Len())
}

func TestLimiter_SweepKeepsActiveBuckets(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 1, IdleTTL: time.Hour})
	mustAllow(l, "client-a")

	assert.Equal(t, 0, l.Sweep(), "还在活跃的桶不应当被清掉")
	assert.Equal(t, 1, l.Len())
}

func TestLimiter_ZeroLimitDeniesEverything(t *testing.T) {
	t.Parallel()

	// 额度为 0 的限流器等于全拒。配置层会拒绝这种值，
	// 这里验证它不会把一个荒谬的等待时长漏给 HTTP 层。
	l := New(Config{Limit: rate.Limit(0), Burst: 1})

	allowed, retryAfter := l.Allow("client-a")
	assert.False(t, allowed)
	assert.Greater(t, retryAfter, time.Duration(0))
	assert.Less(t, retryAfter, time.Minute, "等待时长必须是有限值")
}

func TestLimiter_NormalizesInvalidConfig(t *testing.T) {
	t.Parallel()

	l := New(Config{Limit: rate.Limit(1), Burst: 0})
	assert.Equal(t, 1, l.burst, "burst 至少要是 1")

	l2 := New(Config{Limit: rate.Limit(1), Burst: 2})
	assert.Equal(t, DefaultIdleTTL, l2.idleTTL, "未指定 TTL 时用默认值")
}

func mustAllow(l *Limiter, key string) bool {
	allowed, _ := l.Allow(key)
	return allowed
}
