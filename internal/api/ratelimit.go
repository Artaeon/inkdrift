package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type rateLimiter struct {
	mu         sync.Mutex
	visitors   map[string]*visitor
	rate       int           // max requests
	window     time.Duration // per window
	stop       chan struct{}
	trustProxy bool
}

type visitor struct {
	count   int
	resetAt time.Time
}

func newRateLimiter(rate int, window time.Duration, trustProxy bool) *rateLimiter {
	rl := &rateLimiter{
		visitors:   make(map[string]*visitor),
		rate:       rate,
		window:     window,
		stop:       make(chan struct{}),
		trustProxy: trustProxy,
	}

	// Cleanup stale entries every minute
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				rl.cleanup()
			case <-rl.stop:
				return
			}
		}
	}()

	return rl
}

func (rl *rateLimiter) Close() {
	close(rl.stop)
}

func (rl *rateLimiter) cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	for ip, v := range rl.visitors {
		if now.After(v.resetAt) {
			delete(rl.visitors, ip)
		}
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	v, exists := rl.visitors[ip]

	if !exists || now.After(v.resetAt) {
		rl.visitors[ip] = &visitor{
			count:   1,
			resetAt: now.Add(rl.window),
		}
		return true
	}

	if v.count >= rl.rate {
		return false
	}

	v.count++
	return true
}

// extractIP safely extracts client IP from the request.
// When trustProxy is true (behind a reverse proxy like Traefik/nginx), it reads
// X-Forwarded-For and X-Real-IP headers. When false, it uses RemoteAddr only,
// preventing attackers from spoofing their IP to bypass rate limits.
func extractIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.SplitN(xff, ",", 2)
			ip := strings.TrimSpace(parts[0])
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			ip := strings.TrimSpace(xri)
			if net.ParseIP(ip) != nil {
				return ip
			}
		}
	}

	// Fall back to RemoteAddr, stripping port
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (rl *rateLimiter) middleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r, rl.trustProxy)

		if !rl.allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded, try again later",
			})
			return
		}

		next(w, r)
	}
}
