package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
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
		limiter: newRateLimiter(10, time.Minute), // 10 requests/min per IP
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/v1/subscribe", s.cors(s.limiter.middleware(s.handleSubscribe)))
	s.mux.HandleFunc("GET /api/v1/unsubscribe", s.limiter.middleware(s.handleUnsubscribe))
	s.mux.HandleFunc("POST /api/v1/unsubscribe", s.limiter.middleware(s.handleUnsubscribe))
	s.mux.HandleFunc("GET /api/v1/confirm", s.limiter.middleware(s.handleConfirm))

	// Admin endpoints (API key required)
	s.mux.HandleFunc("GET /api/v1/lists", s.cors(s.auth(s.handleListLists)))
	s.mux.HandleFunc("POST /api/v1/lists", s.cors(s.auth(s.handleCreateList)))
	s.mux.HandleFunc("GET /api/v1/lists/{id}/subscribers", s.cors(s.auth(s.handleListSubscribers)))
	s.mux.HandleFunc("GET /api/v1/campaigns", s.cors(s.auth(s.handleListCampaigns)))
	s.mux.HandleFunc("GET /api/v1/stats", s.cors(s.auth(s.handleStats)))

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
	return srv.ListenAndServe()
}

// Middleware

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

		key := r.Header.Get("X-API-Key")
		if key == "" {
			key = r.Header.Get("Authorization")
			key = strings.TrimPrefix(key, "Bearer ")
		}

		if subtle.ConstantTimeCompare([]byte(key), []byte(s.cfg.API.APIKey)) != 1 {
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

	// Limit name length to prevent abuse
	if len(req.Name) > 200 {
		req.Name = req.Name[:200]
	}

	if req.Email == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email is required"})
		return
	}

	if !emailRegex.MatchString(req.Email) || len(req.Email) > 254 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid email address"})
		return
	}

	// Verify domain has MX records (basic deliverability check)
	parts := strings.SplitN(req.Email, "@", 2)
	if _, err := net.LookupMX(parts[1]); err != nil {
		if _, err := net.LookupHost(parts[1]); err != nil {
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
	if err == nil && existing.Status == "active" {
		writeJSON(w, http.StatusOK, map[string]string{"message": "already subscribed"})
		return
	}

	_, err = s.db.AddSubscriber(req.Email, req.Name, listID)
	if err != nil {
		log.Printf("subscribe error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to subscribe"})
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
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"subscribers": subs,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	})
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := s.db.ListCampaigns()
	if err != nil {
		log.Printf("list campaigns error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}
	writeJSON(w, http.StatusOK, campaigns)
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
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "service": "inkdrift"})
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

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
