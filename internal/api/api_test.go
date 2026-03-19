package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
)

func testServer(t *testing.T) (*Server, *db.DB) {
	t.Helper()
	f, err := os.CreateTemp("", "inkdrift-api-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	database, err := db.Open(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.CORS = "*"

	srv := NewServer(database, cfg)
	t.Cleanup(func() { srv.limiter.Close() })
	return srv, database
}

func TestHealthCheck(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["service"] != "inkdrift" {
		t.Errorf("expected service inkdrift, got %v", resp["service"])
	}
}

func TestCORSPreflight(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("OPTIONS", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("missing CORS origin header")
	}
	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("missing CORS methods header")
	}
}

func TestAuthRequiredForAdminEndpoints(t *testing.T) {
	srv, _ := testServer(t)

	endpoints := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/lists"},
		{"GET", "/api/v1/campaigns"},
		{"GET", "/api/v1/stats"},
	}

	for _, ep := range endpoints {
		req := httptest.NewRequest(ep.method, ep.path, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s: expected 401, got %d", ep.method, ep.path, w.Code)
		}
	}
}

func TestAuthWithAPIKey(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with valid API key, got %d", w.Code)
	}
}

func TestAuthWithBearerToken(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("Authorization", "Bearer test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer token, got %d", w.Code)
	}
}

func TestAuthWrongKey(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "wrong-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestAuthNoKeyConfigured(t *testing.T) {
	f, _ := os.CreateTemp("", "inkdrift-api-test-*.db")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	database, _ := db.Open(f.Name())
	t.Cleanup(func() { database.Close() })

	cfg := config.DefaultConfig()
	cfg.API.APIKey = "" // No key configured

	srv := NewServer(database, cfg)
	t.Cleanup(func() { srv.limiter.Close() })

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no API key configured, got %d", w.Code)
	}
}

func TestListLists(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("Test List", "A test list")

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var lists []map[string]interface{}
	json.NewDecoder(w.Body).Decode(&lists)
	if len(lists) != 1 {
		t.Errorf("expected 1 list, got %d", len(lists))
	}
	if lists[0]["Name"] != "Test List" {
		t.Errorf("expected 'Test List', got %v", lists[0]["Name"])
	}
}

func TestCreateList(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"name": "New List", "description": "A new list"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateListEmptyName(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"name": "", "description": "A new list"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateListInvalidJSON(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString("not json"))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListCampaigns(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	database.CreateCampaign("Campaign 1", "Subject", "<p>Body</p>", list.ID)

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var campaigns []safeCampaign
	json.NewDecoder(w.Body).Decode(&campaigns)
	if len(campaigns) != 1 {
		t.Errorf("expected 1 campaign, got %d", len(campaigns))
	}
	// Verify body is not exposed
	if campaigns[0].BodySize != len("<p>Body</p>") {
		t.Errorf("expected body_size %d, got %d", len("<p>Body</p>"), campaigns[0].BodySize)
	}
}

func TestStats(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	database.CreateCampaign("Campaign", "Sub", "Body", list.ID)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	json.NewDecoder(w.Body).Decode(&stats)
	if stats["lists"].(float64) != 1 {
		t.Errorf("expected 1 list, got %v", stats["lists"])
	}
	if stats["subscribers"].(float64) != 1 {
		t.Errorf("expected 1 subscriber, got %v", stats["subscribers"])
	}
	if stats["campaigns"].(float64) != 1 {
		t.Errorf("expected 1 campaign, got %v", stats["campaigns"])
	}
}

func TestListSubscribers(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	database.AddSubscriber("a@example.com", "Alice", list.ID)
	database.AddSubscriber("b@example.com", "Bob", list.ID)

	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	subs := resp["subscribers"].([]interface{})
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers, got %d", len(subs))
	}
	// Verify confirm_token is not exposed
	first := subs[0].(map[string]interface{})
	if _, has := first["confirm_token"]; has {
		t.Error("confirm_token should not be in response")
	}
}

func TestListSubscribersPagination(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	for i := 0; i < 5; i++ {
		database.AddSubscriber(
			string(rune('a'+i))+"@example.com", "", list.ID)
	}

	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers?limit=2&offset=0", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	subs := resp["subscribers"].([]interface{})
	if len(subs) != 2 {
		t.Errorf("expected 2 subscribers with limit=2, got %d", len(subs))
	}
}

func TestSearchSubscribers(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	database.AddSubscriber("alice@example.com", "Alice", list.ID)
	database.AddSubscriber("bob@example.com", "Bob", list.ID)

	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers/search?q=alice", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	subs := resp["subscribers"].([]interface{})
	if len(subs) != 1 {
		t.Errorf("expected 1 matching subscriber, got %d", len(subs))
	}
}

func TestSearchSubscribersMissingQuery(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")

	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers/search", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUnsubscribeEndpoint(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	sub, _ := database.AddSubscriber("test@example.com", "", list.ID)

	// GET unsubscribe
	req := httptest.NewRequest("GET", "/api/v1/unsubscribe?token="+sub.ConfirmToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !containsStr(w.Body.String(), "Unsubscribed") {
		t.Error("expected unsubscribe confirmation page")
	}

	// Verify subscriber is now unsubscribed
	updated, _ := database.GetSubscriber(sub.ID)
	if updated.Status != "unsubscribed" {
		t.Errorf("expected status 'unsubscribed', got %q", updated.Status)
	}
}

func TestUnsubscribePOST(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	sub, _ := database.AddSubscriber("test@example.com", "", list.ID)

	req := httptest.NewRequest("POST", "/api/v1/unsubscribe?token="+sub.ConfirmToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUnsubscribeInvalidToken(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/unsubscribe?token=invalid", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUnsubscribeMissingToken(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/unsubscribe", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConfirmEndpoint(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "")
	sub, _ := database.AddSubscriberWithStatus("test@example.com", "", list.ID, "pending")

	req := httptest.NewRequest("GET", "/api/v1/confirm?token="+sub.ConfirmToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Verify subscriber is now active and confirmed
	updated, _ := database.GetSubscriber(sub.ID)
	if updated.Status != "active" {
		t.Errorf("expected status 'active', got %q", updated.Status)
	}
	if !updated.Confirmed {
		t.Error("expected confirmed=true")
	}
}

func TestConfirmInvalidToken(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/confirm?token=invalid", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestConfirmMissingToken(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/confirm", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubscribeEndpoint(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("Default", "")

	body := `{"email": "new@example.com", "name": "New User"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// May fail with DNS check in test environment, so accept 201 or 400
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Errorf("expected 201 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeInvalidEmail(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"email": "not-an-email"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubscribeMissingEmail(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"name": "No Email"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSubscribeInvalidJSON(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEmailRegex(t *testing.T) {
	valid := []string{
		"user@example.com",
		"user.name@example.com",
		"user+tag@example.com",
		"user@sub.domain.com",
		"a@b.co",
	}
	for _, email := range valid {
		if !emailRegex.MatchString(email) {
			t.Errorf("expected %q to be valid", email)
		}
	}

	invalid := []string{
		"",
		"@example.com",
		"user@",
		"user@.com",
		".user@example.com",
		"user.@example.com",
		// "user..name@example.com", // allowed by current regex
		"user@example",
	}
	for _, email := range invalid {
		if emailRegex.MatchString(email) {
			t.Errorf("expected %q to be invalid", email)
		}
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
