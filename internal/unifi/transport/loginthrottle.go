package transport

import (
	"context"
	"os"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// defaultLoginInterval is the minimum spacing between login POSTs to one
// controller. The UniFi OS controller rate-limits /api/auth/login (a handful per
// window, then HTTP 429); a burst of fresh sessions — many back-to-back
// `terraform import`s, or the acceptance suite spinning a provider up per test —
// trips it. Spacing the POSTs keeps us under the limit; the Retry-After backoff
// (retryAfter) handles whatever still slips through. It is invisible to a normal
// single-login Configure: the first login to a host never waits. Override or
// disable with UNIFI_LOGIN_MIN_INTERVAL (a Go duration; "0" disables).
const defaultLoginInterval = 1 * time.Second

// loginThrottle spaces login attempts per controller (keyed by base URL),
// process-wide, using a token-bucket rate.Limiter per host: burst 1 so the first
// login to a host is immediate, then refilling one token every interval. One
// controller's logins are spaced; distinct controllers don't block each other
// (each gets its own limiter and window).
type loginThrottle struct {
	mu       sync.Mutex
	interval time.Duration
	limiters map[string]*rate.Limiter
}

// globalLoginThrottle gates every login in the process. Package-global so it
// spans the separate transports the acceptance suite and sweepers build.
var globalLoginThrottle = newLoginThrottle(resolveLoginInterval())

func newLoginThrottle(interval time.Duration) *loginThrottle {
	return &loginThrottle{interval: interval, limiters: map[string]*rate.Limiter{}}
}

// resolveLoginInterval reads UNIFI_LOGIN_MIN_INTERVAL (a Go duration; "0"
// disables spacing), falling back to defaultLoginInterval.
func resolveLoginInterval() time.Duration {
	v := strings.TrimSpace(os.Getenv("UNIFI_LOGIN_MIN_INTERVAL"))
	if v == "" {
		return defaultLoginInterval
	}
	if d, err := time.ParseDuration(v); err == nil && d >= 0 {
		return d
	}
	return defaultLoginInterval
}

// limit translates the configured interval into a rate.Limit: one event per
// interval, or rate.Inf (no spacing) when disabled.
func (l *loginThrottle) limit() rate.Limit {
	if l.interval <= 0 {
		return rate.Inf
	}
	return rate.Every(l.interval)
}

// limiterFor returns the per-host limiter, creating it on first use.
func (l *loginThrottle) limiterFor(host string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.limiters[host]
	if !ok {
		lim = rate.NewLimiter(l.limit(), 1)
		l.limiters[host] = lim
	}
	return lim
}

// wait blocks until another login to host is allowed, returning early (with the
// context error) if ctx is cancelled while waiting.
func (l *loginThrottle) wait(ctx context.Context, host string) error {
	return l.limiterFor(host).Wait(ctx)
}

// setIntervalForTest overrides the spacing and drops cached limiters so the new
// rate takes effect; the unit suites zero it so the many mock logins don't add
// real seconds.
func (l *loginThrottle) setIntervalForTest(d time.Duration) {
	l.mu.Lock()
	l.interval = d
	l.limiters = map[string]*rate.Limiter{}
	l.mu.Unlock()
}
