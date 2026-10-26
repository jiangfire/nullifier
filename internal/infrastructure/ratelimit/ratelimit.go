// Package ratelimit 提供进程内的令牌桶限流器。
//
// 它解决的是「单个来源能打多快」：公开端点上最贵的操作是投票 ——
// 每张票都要验一次 Groth16 proof，不设上限时一个脚本就能把验签算力打满。
//
// 它**不是**一人一票的替代品：资格证明管的是「谁能领」，限流管的是
// 「单个来源能打多快」，两者叠加。
//
// # 键是加盐哈希，不是原文
//
// 限流器的状态常驻内存，比日志更容易被转储、备份或顺手 dump 出来。
// 里面存着明文 IP 是不合适的 —— 仓库已经有一条硬规矩：匿名领取凭证时
// 不记录来源 IP。哈希键把这个口子堵上：盐是每进程随机的，哈希不可逆，
// 所以内存里没有可还原的来源，副本之间的键也不可关联。
package ratelimit

import (
	"crypto/rand"
	"crypto/sha256"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// DefaultIdleTTL 是桶的默认空闲清理阈值。
const DefaultIdleTTL = 10 * time.Minute

// SweepInterval 是清理空闲桶的默认间隔。
const SweepInterval = time.Minute

// Config 是限流器的构造参数。
type Config struct {
	// Limit 是每秒补充的令牌数。
	Limit rate.Limit
	// Burst 是桶容量 —— 允许的突发请求数。
	Burst int
	// IdleTTL 是桶空闲多久之后被清理。0 表示用 DefaultIdleTTL。
	IdleTTL time.Duration
}

// Limiter 是按「键」计数的令牌桶限流器。
//
// 一个 Limiter 对应一个路由组：不同组的额度不同，必须分开计数。
type Limiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
	limit   rate.Limit
	burst   int
	idleTTL time.Duration
	salt    []byte
}

// bucket 是一个来源的令牌桶。
type bucket struct {
	lim      *rate.Limiter
	lastSeen time.Time
}

// New 构造限流器。
func New(cfg Config) *Limiter {
	idleTTL := cfg.IdleTTL
	if idleTTL <= 0 {
		idleTTL = DefaultIdleTTL
	}
	burst := cfg.Burst
	if burst < 1 {
		burst = 1
	}
	return &Limiter{
		buckets: make(map[string]*bucket),
		limit:   cfg.Limit,
		burst:   burst,
		idleTTL: idleTTL,
		salt:    newSalt(),
	}
}

// Allow 判断该键此刻是否还能取用一个令牌。
//
// 返回 (是否放行, 建议等待时长)。等待时长用来填 Retry-After 响应头 ——
// 告诉客户端「多久之后再来」，而不是让它盲目重试。
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l.limit <= 0 {
		// 额度为 0 的限流器等于「全拒」。配置层会拒绝这种值，这里只是防御：
		// 必须返回一个**有限**的等待时长，不能把无限等待漏给 HTTP 层。
		return false, time.Second
	}
	digest := l.digest(key)

	l.mu.Lock()
	defer l.mu.Unlock()

	b, ok := l.buckets[digest]
	if !ok {
		b = &bucket{lim: rate.NewLimiter(l.limit, l.burst)}
		l.buckets[digest] = b
	}
	b.lastSeen = time.Now()

	if b.lim.Allow() {
		return true, 0
	}
	return false, l.retryDelay(b)
}

// Sweep 清理空闲超过 IdleTTL 的桶，返回清理掉的数量。
//
// 桶数随来源数增长，不清理就是内存泄漏。空闲的桶留着也没用 ——
// 它对应的来源这么久没来，早就不在投票窗口里了。
func (l *Limiter) Sweep() int {
	cutoff := time.Now().Add(-l.idleTTL)

	l.mu.Lock()
	defer l.mu.Unlock()

	removed := 0
	for digest, b := range l.buckets {
		if b.lastSeen.Before(cutoff) {
			delete(l.buckets, digest)
			removed++
		}
	}
	return removed
}

// Len 返回当前的桶数（运维与测试用）。
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// StartCleanup 定期清理空闲桶，直到 stop 通道被关闭。
//
// 桶数随来源数增长，不清理就是内存泄漏 —— 而限流器的状态是**常驻内存**的，
// 攻击者轮换来源地址就能把它撑大。
func (l *Limiter) StartCleanup(stop <-chan struct{}) {
	ticker := time.NewTicker(SweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			l.Sweep()
		}
	}
}

// retryDelay 估算「下一个令牌可用」还要多久。
//
// 令牌桶在拒绝时不会把令牌扣成负数，所以缺口是 (1 - 当前令牌数) 个令牌，
// 直接按缺口算。这里**不**走 Reserve/Cancel —— 那套会改动限流器状态，
// 在「已经被拒绝」这条路径上再改一次状态是自找的坑。
func (l *Limiter) retryDelay(b *bucket) time.Duration {
	deficit := 1 - b.lim.Tokens()
	if deficit <= 0 {
		return 0
	}
	return time.Duration(float64(time.Second) * deficit / float64(l.limit))
}

// digest 把原始键算成加盐哈希。
//
// 盐是每个进程启动时随机生成的：哈希不可逆 → 内存里没有明文来源；
// 盐每进程不同 → 多个副本之间的键不可关联，重启后旧键也不再对应。
func (l *Limiter) digest(key string) string {
	h := sha256.New()
	h.Write(l.salt)
	h.Write([]byte{0})
	h.Write([]byte(key))
	return string(h.Sum(nil))
}

// newSalt 生成进程内随机的盐。
func newSalt() []byte {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		// crypto/rand 在现代 Go 上不会失败；真失败了也不能让服务起不来 ——
		// 退化成固定盐：哈希依然不可逆，只是键可以跨进程关联。
		return []byte("nullifier-ratelimit-fallback")
	}
	return salt
}
