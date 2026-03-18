package api

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

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
		limiter: newRateLimiter(10, time.Minute), // 10 subscribes per minute per IP
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("POST /api/v1/subscribe", s.cors(s.limiter.middleware(s.handleSubscribe)))
	s.mux.HandleFunc("GET /api/v1/unsubscribe", s.handleUnsubscribe)
	s.mux.HandleFunc("POST /api/v1/unsubscribe", s.handleUnsubscribe)
	s.mux.HandleFunc("GET /api/v1/confirm", s.handleConfirm)

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
	return http.ListenAndServe(addr, s.mux)
}

// Middleware

func (s *Server) cors(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := s.cfg.API.CORS
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
			next(w, r)
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to subscribe"})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{"message": "subscribed successfully"})
}

func (s *Server) handleUnsubscribe(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
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
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list"})
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
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}

	list, err := s.db.CreateList(req.Name, req.Description)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create list"})
		return
	}

	writeJSON(w, http.StatusCreated, list)
}

func (s *Server) handleListSubscribers(w http.ResponseWriter, r *http.Request) {
	listID := r.PathValue("id")
	subs, err := s.db.ListSubscribers(listID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list subscribers"})
		return
	}
	writeJSON(w, http.StatusOK, subs)
}

func (s *Server) handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	campaigns, err := s.db.ListCampaigns()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list campaigns"})
		return
	}
	writeJSON(w, http.StatusOK, campaigns)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	lists, _ := s.db.ListLists()
	campaigns, _ := s.db.ListCampaigns()

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
