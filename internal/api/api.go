package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
	"github.com/artaeon/inkdrift/internal/smtp"
)

// RFC 5322 simplified — rejects consecutive dots, leading/trailing dots
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._%+\-]*[a-zA-Z0-9])?@[a-zA-Z0-9]([a-zA-Z0-9.\-]*[a-zA-Z0-9])?\.[a-zA-Z]{2,}$`)

type Server struct {
	db      *db.DB
	cfg     *config.Config
	mux     *http.ServeMux
	limiter *rateLimiter
}

func NewServer(database *db.DB, cfg *config.Config) *Server {
	s := &Server{
		db:      database,
		cfg:     cfg,
		mux:     http.NewServeMux(),
		limiter: newRateLimiter(max(cfg.API.RateLimit, 10), time.Minute, cfg.API.TrustProxy),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/v1/subscribe", s.logged(s.cors(s.limiter.middleware(s.handleSubscribe))))
	s.mux.HandleFunc("GET /api/v1/unsubscribe", s.logged(s.limiter.middleware(s.handleUnsubscribe)))
	s.mux.HandleFunc("POST /api/v1/unsubscribe", s.logged(s.limiter.middleware(s.handleUnsubscribe)))
	s.mux.HandleFunc("GET /api/v1/confirm", s.logged(s.limiter.middleware(s.handleConfirm)))

	// Admin endpoints (API key required)
	s.mux.HandleFunc("GET /api/v1/lists", s.logged(s.cors(s.auth(s.handleListLists))))
	s.mux.HandleFunc("POST /api/v1/lists", s.logged(s.cors(s.auth(s.handleCreateList))))
	s.mux.HandleFunc("GET /api/v1/lists/{id}/subscribers", s.logged(s.cors(s.auth(s.handleListSubscribers))))
	s.mux.HandleFunc("GET /api/v1/lists/{id}/subscribers/search", s.logged(s.cors(s.auth(s.handleSearchSubscribers))))
	s.mux.HandleFunc("GET /api/v1/campaigns", s.logged(s.cors(s.auth(s.handleListCampaigns))))
	s.mux.HandleFunc("GET /api/v1/stats", s.logged(s.cors(s.auth(s.handleStats))))

	// Health check
	s.mux.HandleFunc("GET /health", s.handleHealth)

	// CORS preflight
	s.mux.HandleFunc("OPTIONS /", s.handleOptions)
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) ListenAndServe() error {
	addr := fmt.Sprintf("%s:%d", s.cfg.API.Host, s.cfg.API.Port)
	log.Printf("InkDrift API listening on %s", addr)

	srv := &http.Server{
		Addr:              addr,
		Handler:           s.mux,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 16, // 64KB
	}

	// Graceful shutdown on interrupt
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errCh:
		s.limiter.Close()
		return err
	case sig := <-quit:
		log.Printf("Received %s, shutting down gracefully...", sig)
		s.limiter.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

// Middleware

// statusWriter wraps ResponseWriter to capture the status code for logging
type statusWriter struct {
	http.ResponseWriter
	code int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.code = code
	sw.ResponseWriter.WriteHeader(code)
}

func (s *Server) logged(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
		next(sw, r)
		log.Printf("%s %s %d %s %s", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Millisecond), extractIP(r, s.cfg.API.TrustProxy))
	}
}

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := s.cfg.API.CORS
		if origin == "" {
			origin = "localhost"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
		w.Header().Set("Access-Control-Max-Age", "86400")
		next(w, r)
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.API.APIKey == "" {
			// No API key configured — block admin endpoints entirely
			writeJSON(w, http.StatusForbidden, map[string]string{
				"error": "admin API key not configured, set api_key in config or INKDRIFT_API_KEY env",
			})
			return
		}

		// Rate limit auth attempts to prevent brute force
		ip := extractIP(r, s.cfg.API.TrustProxy)
		if !s.limiter.allow(ip) {
			w.Header().Set("Retry-After", "60")
			writeJSON(w, http.StatusTooManyRequests, map[string]string{
				"error": "rate limit exceeded, try again later",
			})
			return
		}

		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.Header.Get("Authorization")
			key = strings.TrimPrefix(key, "Bearer ")
		}

		if subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.API.APIKey)) != 1 {
			log.Printf("auth fail: %s %s from %s", r.Method, r.URL.Path, ip)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// Handlers

func (s *Server) handleSubscribe(w http.ResponseWriter, r *http.Request) {
	// Limit request body to 1KB to prevent abuse
	r.Body = http.MaxBytesReader(w, r.Body, 1024)

	var req struct {
		Email  string `json:"email"`
		Name   string `json:"name"`
		ListID string `json:"list_id"`
		List   string `json:"list"` // list name alternative
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	// Limit name length to prevent abuse (truncate at rune boundary for valid UTF-8)
	if len(req.Name) > 200 {
		runes := []rune(req.Name)
		if len(runes) > 200 {
			req.Name = string(runes[:200])
		}
	}

	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	if !emailRegex.MatchString(req.Email) || len(req.Email) > 254 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email address"})
		return
	}

	// Verify domain has MX records (basic deliverability check) with timeout
	parts := strings.SplitN(req.Email, "@", 2)
	resolver := &net.Resolver{}
	dnsCtx, dnsCancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer dnsCancel()
	if _, err := resolver.LookupMX(dnsCtx, parts[1]); err != nil {
		if _, err := resolver.LookupHost(dnsCtx, parts[1]); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email domain"})
			return
		}
	}

	// Resolve list
	listID := req.ListID
	if listID == "" && req.List != "" {
		list, err := s.db.GetListByName(req.List)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "list not found"})
			return
		}
		listID = list.ID
	}

	if listID == "" {
		// Use first list as default
		lists, err := s.db.ListLists()
		if err != nil || len(lists) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no lists available"})
			return
		}
		listID = lists[0].ID
	}

	// Check if already subscribed
	existing, err := s.db.GetSubscriberByEmail(req.Email, listID)
	if err == nil {
		if existing.Status == "active" {
			writeJSON(w, http.StatusOK, map[string]string{"message": "already subscribed"})
			return
		}
		if existing.Status == "pending" {
			writeJSON(w, http.StatusOK, map[string]string{"message": "confirmation email already sent, please check your inbox"})
			return
		}
		// Re-subscribe: unsubscribed or bounced users can sign up again
		if existing.Status == "unsubscribed" || existing.Status == "bounced" {
			if s.cfg.SMTPConfigured() && s.cfg.Server.Domain != "" {
				if err := s.db.ResubscribePending(existing.ID); err != nil {
					log.Printf("resubscribe error: %v", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resubscribe"})
					return
				}
				// Re-fetch to get the new confirm token generated by ResubscribePending
				refreshed, err := s.db.GetSubscriber(existing.ID)
				if err != nil {
					log.Printf("resubscribe fetch error: %v", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resubscribe"})
					return
				}
				go s.sendConfirmationEmail(refreshed)
				writeJSON(w, http.StatusOK, map[string]string{"message": "please check your email to confirm your subscription"})
			} else {
				if err := s.db.ResubscribeActive(existing.ID); err != nil {
					log.Printf("resubscribe error: %v", err)
					writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resubscribe"})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"message": "resubscribed successfully"})
			}
			return
		}
	}

	// If SMTP is configured and double opt-in is possible, create as pending
	status := "active"
	if s.cfg.SMTPConfigured() && s.cfg.Server.Domain != "" {
		status = "pending"
	}

	sub, err := s.db.AddSubscriberWithStatus(req.Email, req.Name, listID, status)
	if err != nil {
		log.Printf("subscribe error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to subscribe"})
		return
	}

	// Send confirmation email for double opt-in
	if status == "pending" {
		go s.sendConfirmationEmail(sub)
		writeJSON(w, http.StatusCreated, map[string]string{"message": "please check your email to confirm your subscription"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "subscribed successfully"})
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > 128 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.db.UnsubscribeByToken(token); err != nil {
		http.Error(w, "Unsubscribe failed", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:50px">
<h2>Unsubscribed</h2><p>You have been successfully unsubscribed.</p></body></html>`)
}

func (s *Server) handleConfirm(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" || len(token) > 128 {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if err := s.db.ConfirmSubscriber(token); err != nil {
		http.Error(w, "Confirmation failed", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!DOCTYPE html><html><body style="font-family:sans-serif;text-align:center;padding:50px">
<h2>Confirmed!</h2><p>Your subscription has been confirmed. Thank you!</p></body></html>`)
}

func (s *Server) handleListLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.db.ListLists()
	if err != nil {
		log.Printf("list lists error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	type listWithCount struct {
		db.List
		SubscriberCount int `json:"subscriber_count"`
	}

	var result []listWithCount
	for _, l := range lists {
		count, _ := s.db.ListSubscriberCount(l.ID)
		result = append(result, listWithCount{List: l, SubscriberCount: count})
	}

	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleCreateList(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)

	if req.Name == "" || len(req.Name) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required (max 200 chars)"})
		return
	}
	if len(req.Description) > 1000 {
		req.Description = req.Description[:1000]
	}

	list, err := s.db.CreateList(req.Name, req.Description)
	if err != nil {
		log.Printf("create list error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create list"})
		return
	}

	writeJSON(w, http.StatusCreated, list)
}

// safeSubscriber is a subscriber without sensitive fields for API responses
type safeSubscriber struct {
	ID           string  `json:"id"`
	Email        string  `json:"email"`
	Name         string  `json:"name"`
	ListID       string  `json:"list_id"`
	Status       string  `json:"status"`
	Confirmed    bool    `json:"confirmed"`
	SubscribedAt string  `json:"subscribed_at"`
}

func (s *Server) handleListSubscribers(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	if listID == "" || len(listID) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid list ID"})
		return
	}

	// Pagination
	limit := 100
	offset := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 && n <= 1000000 {
			offset = n
		}
	}

	subs, err := s.db.ListSubscribersPaginated(listID, limit, offset)
	if err != nil {
		log.Printf("list subscribers error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	total, _ := s.db.ListSubscriberCount(listID)

	// Strip sensitive fields (confirm_token, metadata) from response
	safe := make([]safeSubscriber, len(subs))
	for i, sub := range subs {
		safe[i] = safeSubscriber{
			ID:           sub.ID,
			Email:        sub.Email,
			Name:         sub.Name,
			ListID:       sub.ListID,
			Status:       sub.Status,
			Confirmed:    sub.Confirmed,
			SubscribedAt: sub.SubscribedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscribers": safe,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (s *Server) handleSearchSubscribers(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	if listID == "" || len(listID) > 64 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid list ID"})
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" || len(query) > 200 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "query parameter 'q' is required (max 200 chars)"})
		return
	}

	subs, err := s.db.SearchSubscribers(listID, query, 50)
	if err != nil {
		log.Printf("search subscribers error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	safe := make([]safeSubscriber, len(subs))
	for i, sub := range subs {
		safe[i] = safeSubscriber{
			ID:           sub.ID,
			Email:        sub.Email,
			Name:         sub.Name,
			ListID:       sub.ListID,
			Status:       sub.Status,
			Confirmed:    sub.Confirmed,
			SubscribedAt: sub.SubscribedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscribers": safe,
		"total":       len(safe),
	})
}

type safeCampaign struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Subject     string  `json:"subject"`
	ListID      string  `json:"list_id"`
	Status      string  `json:"status"`
	TemplateID  string  `json:"template_id,omitempty"`
	SentAt      *string `json:"sent_at,omitempty"`
	SentCount   int     `json:"sent_count"`
	FailedCount int     `json:"failed_count"`
	BodySize    int     `json:"body_size"`
	CreatedAt   string  `json:"created_at"`
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := s.db.ListCampaigns()
	if err != nil {
		log.Printf("list campaigns error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	safe := make([]safeCampaign, len(campaigns))
	for i, c := range campaigns {
		safe[i] = safeCampaign{
			ID:          c.ID,
			Name:        c.Name,
			Subject:     c.Subject,
			ListID:      c.ListID,
			Status:      c.Status,
			TemplateID:  c.TemplateID,
			SentCount:   c.SentCount,
			FailedCount: c.FailedCount,
			BodySize:    len(c.Body),
			CreatedAt:   c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if c.SentAt != nil {
			s := c.SentAt.Format("2006-01-02T15:04:05Z")
			safe[i].SentAt = &s
		}
	}
	writeJSON(w, http.StatusOK, safe)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	lists, err := s.db.ListLists()
	if err != nil {
		log.Printf("stats error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	campaigns, err := s.db.ListCampaigns()
	if err != nil {
		log.Printf("stats error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	totalSubscribers := 0
	for _, l := range lists {
		count, _ := s.db.ListSubscriberCount(l.ID)
		totalSubscribers += count
	}

	totalSent := 0
	for _, c := range campaigns {
		totalSent += c.SentCount
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"lists":       len(lists),
		"subscribers": totalSubscribers,
		"campaigns":   len(campaigns),
		"emails_sent": totalSent,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Actually verify the database is accessible
	if _, err := s.db.ListLists(); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status":  "error",
			"service": "inkdrift",
			"error":   "database unavailable",
		})
		return
	}

	result := map[string]interface{}{
		"status":  "ok",
		"service": "inkdrift",
		"smtp":    s.cfg.SMTPConfigured(),
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	origin := s.cfg.API.CORS
	if origin == "" {
		origin = "localhost"
	}
	w.Header().Set("Access-Control-Allow-Origin", origin)
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")
	w.Header().Set("Access-Control-Max-Age", "86400")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) sendConfirmationEmail(sub *db.Subscriber) {
	domain := s.cfg.Server.Domain
	scheme := "https"
	if domain == "" {
		domain = fmt.Sprintf("localhost:%d", s.cfg.API.Port)
		scheme = "http"
	}
	confirmURL := fmt.Sprintf("%s://%s/api/v1/confirm?token=%s", scheme, domain, sub.ConfirmToken)

	sender := smtp.NewSender(s.cfg.SMTP)
	err := sender.Send(smtp.Email{
		To:      sub.Email,
		Subject: fmt.Sprintf("Confirm your subscription to %s", s.cfg.Server.Name),
		HTML: fmt.Sprintf(`<div style="font-family:sans-serif;max-width:500px;margin:0 auto;padding:20px">
<h2>Confirm your subscription</h2>
<p>You've been asked to subscribe to <strong>%s</strong>.</p>
<p>Click the button below to confirm:</p>
<p style="text-align:center;margin:30px 0">
  <a href="%s" style="background:#6366f1;color:white;padding:12px 30px;text-decoration:none;border-radius:6px;font-weight:bold">Confirm Subscription</a>
</p>
<p style="color:#666;font-size:13px">If you didn't request this, you can safely ignore this email.</p>
<p style="color:#999;font-size:12px">Or copy this link: %s</p>
</div>`, s.cfg.Server.Name, confirmURL, confirmURL),
		Text: fmt.Sprintf("Confirm your subscription to %s\n\nClick here to confirm: %s\n\nIf you didn't request this, ignore this email.",
			s.cfg.Server.Name, confirmURL),
	})
	if err != nil {
		log.Printf("failed to send confirmation email to %s: %v", sub.Email, err)
	}
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}
