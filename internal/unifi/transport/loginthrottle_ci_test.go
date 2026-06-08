package transport

import (
	"context"
	"testing"
	"time"
)

// TestLoginThrottleSpacesPerHost: successive logins to one host are spaced by at
// least the interval; a different host is not held back by another's schedule.
func TestLoginThrottleSpacesPerHost(t *testing.T) {
	thr := newLoginThrottle(40 * time.Millisecond)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 3; i++ {
		if err := thr.wait(ctx, "https://a"); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	// 3 logins (burst 1) => the 2nd and 3rd each wait one interval: ~80ms total.
	// Assert a margin under that to stay robust against timer measurement.
	if elapsed := time.Since(start); elapsed < 70*time.Millisecond {
		t.Errorf("3 spaced logins took %v, want >= ~70ms", elapsed)
	}

	// A first login to a fresh host is immediate (not blocked by host "a").
	start2 := time.Now()
	if err := thr.wait(ctx, "https://b"); err != nil {
		t.Fatalf("wait: %v", err)
	}
	if elapsed := time.Since(start2); elapsed > 30*time.Millisecond {
		t.Errorf("first login to a new host waited %v, want ~immediate", elapsed)
	}
}

// TestLoginThrottleDisabled: a zero interval never waits.
func TestLoginThrottleDisabled(t *testing.T) {
	thr := newLoginThrottle(0)
	start := time.Now()
	for i := 0; i < 5; i++ {
		if err := thr.wait(context.Background(), "h"); err != nil {
			t.Fatalf("wait: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 30*time.Millisecond {
		t.Errorf("disabled throttle waited %v, want ~0", elapsed)
	}
}

// TestLoginThrottleContextCancel: a pending wait returns the context error
// rather than sleeping out a long interval.
func TestLoginThrottleContextCancel(t *testing.T) {
	thr := newLoginThrottle(time.Hour)
	if err := thr.wait(context.Background(), "h"); err != nil { // first reserves the slot
		t.Fatalf("first wait: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := thr.wait(ctx, "h"); err == nil { // second would wait ~1h
		t.Error("want a context-cancellation error on the long wait, got nil")
	}
}

// TestResolveLoginInterval covers env parsing.
func TestResolveLoginInterval(t *testing.T) {
	t.Setenv("UNIFI_LOGIN_MIN_INTERVAL", "250ms")
	if got := resolveLoginInterval(); got != 250*time.Millisecond {
		t.Errorf("interval = %v, want 250ms", got)
	}
	t.Setenv("UNIFI_LOGIN_MIN_INTERVAL", "0")
	if got := resolveLoginInterval(); got != 0 {
		t.Errorf("interval = %v, want 0 (disabled)", got)
	}
	t.Setenv("UNIFI_LOGIN_MIN_INTERVAL", "garbage")
	if got := resolveLoginInterval(); got != defaultLoginInterval {
		t.Errorf("interval = %v, want default %v on a bad value", got, defaultLoginInterval)
	}
}
