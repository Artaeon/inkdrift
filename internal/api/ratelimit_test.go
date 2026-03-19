package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllow(t *testing.T) {
	rl := newRateLimiter(3, time.Minute, false)
	defer rl.Close()

	// First 3 requests should be allowed
	for i := 0; i < 3; i++ {
		if !rl.allow("192.168.1.1") {
			t.Errorf("request %d should be allowed", i+1)
		}
	}

	// 4th request should be blocked
	if rl.allow("192.168.1.1") {
		t.Error("4th request should be blocked")
	}

	// Different IP should still be allowed
	if !rl.allow("192.168.1.2") {
		t.Error("different IP should be allowed")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	rl := newRateLimiter(1, 50*time.Millisecond, false)
	defer rl.Close()

	if !rl.allow("1.2.3.4") {
		t.Error("first request should be allowed")
	}
	if rl.allow("1.2.3.4") {
		t.Error("second request should be blocked")
	}

	// Wait for window to expire
	time.Sleep(60 * time.Millisecond)

	if !rl.allow("1.2.3.4") {
		t.Error("request after window reset should be allowed")
	}
}

func TestRateLimiterCleanup(t *testing.T) {
	rl := newRateLimiter(1, 10*time.Millisecond, false)
	defer rl.Close()

	rl.allow("1.2.3.4")
	time.Sleep(20 * time.Millisecond)

	rl.cleanup()

	rl.mu.Lock()
	_, exists := rl.visitors["1.2.3.4"]
	rl.mu.Unlock()

	if exists {
		t.Error("expired visitor should be cleaned up")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := newRateLimiter(2, time.Minute, false)
	defer rl.Close()

	handler := rl.middleware(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First 2 requests pass through
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:1234"
		w := httptest.NewRecorder()
		handler(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, w.Code)
		}
	}

	// 3rd request is rate limited
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "60" {
		t.Error("missing Retry-After header")
	}
}

func TestExtractIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		xff        string
		xri        string
		trustProxy bool
		expected   string
	}{
		{
			name:       "remote addr with port",
			remoteAddr: "192.168.1.1:1234",
			trustProxy: false,
			expected:   "192.168.1.1",
		},
		{
			name:       "remote addr without port",
			remoteAddr: "192.168.1.1",
			trustProxy: false,
			expected:   "192.168.1.1",
		},
		{
			name:       "XFF ignored when trust_proxy false",
			remoteAddr: "192.168.1.1:1234",
			xff:        "10.0.0.1, 10.0.0.2",
			trustProxy: false,
			expected:   "192.168.1.1",
		},
		{
			name:       "XFF trusted",
			remoteAddr: "192.168.1.1:1234",
			xff:        "10.0.0.1, 10.0.0.2",
			trustProxy: true,
			expected:   "10.0.0.1",
		},
		{
			name:       "X-Real-IP trusted",
			remoteAddr: "192.168.1.1:1234",
			xri:        "10.0.0.5",
			trustProxy: true,
			expected:   "10.0.0.5",
		},
		{
			name:       "XFF takes precedence over X-Real-IP",
			remoteAddr: "192.168.1.1:1234",
			xff:        "10.0.0.1",
			xri:        "10.0.0.5",
			trustProxy: true,
			expected:   "10.0.0.1",
		},
		{
			name:       "invalid XFF falls through to X-Real-IP",
			remoteAddr: "192.168.1.1:1234",
			xff:        "not-an-ip",
			xri:        "10.0.0.5",
			trustProxy: true,
			expected:   "10.0.0.5",
		},
		{
			name:       "invalid XFF and XRI falls through to remote addr",
			remoteAddr: "192.168.1.1:1234",
			xff:        "not-an-ip",
			xri:        "also-not-an-ip",
			trustProxy: true,
			expected:   "192.168.1.1",
		},
		{
			name:       "IPv6 remote addr",
			remoteAddr: "[::1]:1234",
			trustProxy: false,
			expected:   "::1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}

			got := extractIP(req, tt.trustProxy)
			if got != tt.expected {
				t.Errorf("extractIP() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestStatusWriter(t *testing.T) {
	w := httptest.NewRecorder()
	sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}

	sw.WriteHeader(http.StatusNotFound)
	if sw.code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", sw.code)
	}
}
