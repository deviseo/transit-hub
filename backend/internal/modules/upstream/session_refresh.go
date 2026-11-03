package upstream

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// rotatedSessionTTL 是「旧 refresh token -> 轮换后的会话」记录在内存中的保留时长。
// 它需要覆盖持有旧会话副本的调用方可能出现的最大滞后：new-api 的 access token 只有 15 分钟，
// 30 分钟至少覆盖两代轮换；更早的 refresh token 即使被重放，new-api 也只会拒绝，不会吊销会话。
const rotatedSessionTTL = 30 * time.Minute

// maxRotationHops 限制沿轮换记录追溯的步数，防御异常数据形成环。
const maxRotationHops = 16

// errSessionRefreshAborted 只在兑换函数异常退出（panic）时返回给并发等待者，
// 确保等待者不会拿到「空会话 + nil 错误」。
var errSessionRefreshAborted = newRequestError(ErrorUnknown, "")

// sessionRefresher 保证同一个 refresh token 在本进程内只向上游兑换一次：
//   - 并发刷新合并为一次上游请求（single-flight）；
//   - 兑换结果保留一段时间。持有旧会话副本的调用方（Redis 与 PostgreSQL 各存一份的 admin 会话、
//     排队等待的批量任务等）再用旧 refresh token 刷新时，直接拿到已经轮换出的最新会话，
//     不再把旧 token 发给上游：sub2api 会拒绝已删除的旧 token；new-api 会把 30 秒容错窗口之后
//     重新出现的上一代 token 视为重放，直接吊销整个登录会话。
//
// 该保护只覆盖单个 TransitHub 进程；进程重启后内存记录清空，所以调用方仍必须持久化刷新结果。
type sessionRefresher struct {
	mu       sync.Mutex
	inflight map[string]*refreshCall
	rotated  map[string]rotatedSession
	now      func() time.Time
}

type refreshCall struct {
	done    chan struct{}
	session Session
	err     error
}

type rotatedSession struct {
	session   Session
	rotatedAt time.Time
}

func newSessionRefresher() *sessionRefresher {
	return &sessionRefresher{
		inflight: make(map[string]*refreshCall),
		rotated:  make(map[string]rotatedSession),
		now:      time.Now,
	}
}

// refreshTokenKey 用摘要而不是 refresh token 明文作为内存索引键。
func refreshTokenKey(session Session) string {
	sum := sha256.Sum256([]byte(string(session.Platform) + "\x00" + session.BaseURL + "\x00" + session.RefreshToken))
	return hex.EncodeToString(sum[:])
}

// refresh 先沿轮换记录把 session 追到本进程已知的最新版本；最新版本仍需刷新时，
// 合并同一 refresh token 的并发请求，只调用一次 exchange。
func (r *sessionRefresher) refresh(session Session, needsRefresh func(Session) bool, exchange func(Session) (Session, error)) (Session, error) {
	r.mu.Lock()
	r.pruneLocked()
	if latest, ok := r.latestLocked(session); ok {
		session = latest
		if !needsRefresh(session) {
			r.mu.Unlock()
			return session, nil
		}
	}
	key := refreshTokenKey(session)
	if call, ok := r.inflight[key]; ok {
		r.mu.Unlock()
		<-call.done
		return call.session, call.err
	}
	call := &refreshCall{done: make(chan struct{}), err: errSessionRefreshAborted}
	r.inflight[key] = call
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.inflight, key)
		if call.err == nil && call.session.RefreshToken != "" && call.session.RefreshToken != session.RefreshToken {
			r.rotated[key] = rotatedSession{session: call.session, rotatedAt: r.now()}
		}
		r.mu.Unlock()
		close(call.done)
	}()
	call.session, call.err = exchange(session)
	return call.session, call.err
}

// latestLocked 沿「旧 refresh token -> 新会话」记录追到最新会话；ok=false 表示该 token 未被本进程轮换过。
func (r *sessionRefresher) latestLocked(session Session) (Session, bool) {
	entry, ok := r.rotated[refreshTokenKey(session)]
	if !ok {
		return Session{}, false
	}
	latest := entry.session
	for hop := 0; hop < maxRotationHops; hop++ {
		next, ok := r.rotated[refreshTokenKey(latest)]
		if !ok {
			break
		}
		latest = next.session
	}
	return latest, true
}

func (r *sessionRefresher) pruneLocked() {
	cutoff := r.now().Add(-rotatedSessionTTL)
	for key, entry := range r.rotated {
		if entry.rotatedAt.Before(cutoff) {
			delete(r.rotated, key)
		}
	}
}
