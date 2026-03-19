package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func testServerWithConfig(t *testing.T, cfg *config.Config) (*Server, *db.DB) {
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

func TestHealthCheckSMTPStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.SMTP.Host = "smtp.example.com"
	cfg.SMTP.From = "test@example.com"
	srv, _ := testServerWithConfig(t, cfg)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["smtp"] != true {
		t.Error("expected smtp=true when configured")
	}
}

func TestHealthCheckDBClosed(t *testing.T) {
	f, _ := os.CreateTemp("", "inkdrift-api-test-*.db")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	database, _ := db.Open(f.Name())
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test"
	srv := NewServer(database, cfg)
	t.Cleanup(func() { srv.limiter.Close() })

	// Close DB to simulate unavailable
	database.Close()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
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
	if w.Header().Get("Access-Control-Max-Age") != "86400" {
		t.Error("missing CORS max-age header")
	}
}

func TestCORSDefaultOrigin(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.CORS = "" // empty CORS
	srv, _ := testServerWithConfig(t, cfg)

	req := httptest.NewRequest("OPTIONS", "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "localhost" {
		t.Errorf("expected default CORS 'localhost', got %q", w.Header().Get("Access-Control-Allow-Origin"))
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
	cfg := config.DefaultConfig()
	cfg.API.APIKey = ""
	srv, _ := testServerWithConfig(t, cfg)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when no API key configured, got %d", w.Code)
	}
}

func TestListListsEmpty(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListListsWithSubscriberCount(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test List", "A test list", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	database.AddSubscriber("b@example.com", "", list.ID)

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
		t.Fatalf("expected 1 list, got %d", len(lists))
	}
	if lists[0]["subscriber_count"].(float64) != 2 {
		t.Errorf("expected subscriber_count 2, got %v", lists[0]["subscriber_count"])
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

func TestCreateListWithFromIdentity(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"name": "Site A", "description": "First site", "from_email": "news@site-a.com", "from_name": "Site A News"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateListInvalidFromEmail(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"name": "Bad Email", "from_email": "not-an-email"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid from_email, got %d", w.Code)
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

func TestCreateListLongName(t *testing.T) {
	srv, _ := testServer(t)

	longName := strings.Repeat("a", 201)
	body := `{"name": "` + longName + `"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long name, got %d", w.Code)
	}
}

func TestCreateListLongDescription(t *testing.T) {
	srv, _ := testServer(t)

	longDesc := strings.Repeat("d", 1500)
	body := `{"name": "Test", "description": "` + longDesc + `"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should succeed (description is truncated, not rejected)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201 (desc truncated), got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateListDuplicate(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("Existing", "", "", "")

	body := `{"name": "Existing"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for duplicate list, got %d", w.Code)
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

func TestListCampaignsEmpty(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestListCampaignsWithSentAt(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	c, _ := database.CreateCampaign("Campaign", "Subject", "<p>Body</p>", list.ID)
	// Set sent_at via UpdateCampaignStats
	database.UpdateCampaignStats(c.ID, 5, 0)

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
		t.Fatalf("expected 1 campaign, got %d", len(campaigns))
	}
	if campaigns[0].SentAt == nil {
		t.Error("expected sent_at to be populated")
	}
	if campaigns[0].BodySize != len("<p>Body</p>") {
		t.Errorf("expected body_size %d, got %d", len("<p>Body</p>"), campaigns[0].BodySize)
	}
}

func TestStatsEmpty(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var stats map[string]interface{}
	json.NewDecoder(w.Body).Decode(&stats)
	if stats["lists"].(float64) != 0 {
		t.Errorf("expected 0 lists, got %v", stats["lists"])
	}
	if stats["emails_sent"].(float64) != 0 {
		t.Errorf("expected 0 emails_sent, got %v", stats["emails_sent"])
	}
}

func TestStatsWithData(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Campaign", "Sub", "Body", list.ID)
	database.UpdateCampaignStats(c.ID, 5, 1)

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
	if stats["emails_sent"].(float64) != 5 {
		t.Errorf("expected 5 emails_sent, got %v", stats["emails_sent"])
	}
}

func TestListSubscribers(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
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
	// Verify metadata is not exposed
	if _, has := first["metadata"]; has {
		t.Error("metadata should not be in response")
	}
	// Verify expected fields are present
	if _, has := first["id"]; !has {
		t.Error("id should be in response")
	}
	if _, has := first["subscribed_at"]; !has {
		t.Error("subscribed_at should be in response")
	}
}

func TestListSubscribersInvalidListID(t *testing.T) {
	srv, _ := testServer(t)

	// Very long list ID
	longID := strings.Repeat("x", 100)
	req := httptest.NewRequest("GET", "/api/v1/lists/"+longID+"/subscribers", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long list ID, got %d", w.Code)
	}
}

func TestListSubscribersPagination(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
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
	if resp["total"].(float64) != 5 {
		t.Errorf("expected total 5, got %v", resp["total"])
	}
	if resp["limit"].(float64) != 2 {
		t.Errorf("expected limit 2, got %v", resp["limit"])
	}
	if resp["offset"].(float64) != 0 {
		t.Errorf("expected offset 0, got %v", resp["offset"])
	}
}

func TestListSubscribersInvalidPagination(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)

	// Invalid limit and offset should use defaults
	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers?limit=abc&offset=xyz", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with invalid pagination, got %d", w.Code)
	}
}

func TestSearchSubscribers(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
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
	if resp["total"].(float64) != 1 {
		t.Errorf("expected total 1, got %v", resp["total"])
	}
}

func TestSearchSubscribersMissingQuery(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")

	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers/search", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchSubscribersLongQuery(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")

	longQuery := strings.Repeat("a", 201)
	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers/search?q="+longQuery, nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long query, got %d", w.Code)
	}
}

func TestSearchSubscribersInvalidListID(t *testing.T) {
	srv, _ := testServer(t)

	longID := strings.Repeat("x", 100)
	req := httptest.NewRequest("GET", "/api/v1/lists/"+longID+"/subscribers/search?q=test", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long list ID, got %d", w.Code)
	}
}

func TestUnsubscribeEndpoint(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriber("test@example.com", "", list.ID)

	req := httptest.NewRequest("GET", "/api/v1/unsubscribe?token="+sub.ConfirmToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Unsubscribed") {
		t.Error("expected unsubscribe confirmation page")
	}

	updated, _ := database.GetSubscriber(sub.ID)
	if updated.Status != "unsubscribed" {
		t.Errorf("expected status 'unsubscribed', got %q", updated.Status)
	}
}

func TestUnsubscribePOST(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
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

func TestUnsubscribeLongToken(t *testing.T) {
	srv, _ := testServer(t)

	longToken := strings.Repeat("a", 200)
	req := httptest.NewRequest("GET", "/api/v1/unsubscribe?token="+longToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long token, got %d", w.Code)
	}
}

func TestConfirmEndpoint(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriberWithStatus("test@example.com", "", list.ID, "pending")

	req := httptest.NewRequest("GET", "/api/v1/confirm?token="+sub.ConfirmToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Confirmed") {
		t.Error("expected confirm page")
	}

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

func TestConfirmLongToken(t *testing.T) {
	srv, _ := testServer(t)

	longToken := strings.Repeat("a", 200)
	req := httptest.NewRequest("GET", "/api/v1/confirm?token="+longToken, nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long token, got %d", w.Code)
	}
}

func TestSubscribeEndpoint(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("Default", "", "", "")

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

func TestSubscribeLongEmail(t *testing.T) {
	srv, _ := testServer(t)

	longEmail := strings.Repeat("a", 250) + "@b.co"
	body := `{"email": "` + longEmail + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for long email, got %d", w.Code)
	}
}

func TestSubscribeAlreadyActive(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("existing@example.com", "", list.ID)

	body := `{"email": "existing@example.com", "list_id": "` + list.ID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// DNS check might fail, but if it passes should return 200 "already subscribed"
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeAlreadyPending(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriberWithStatus("pending@example.com", "", list.ID, "pending")

	body := `{"email": "pending@example.com", "list_id": "` + list.ID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeResubscribeNoSMTP(t *testing.T) {
	// Without SMTP configured, resubscribe should go directly to active
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	// SMTP not configured, domain not set
	srv, database := testServerWithConfig(t, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriber("unsub@example.com", "", list.ID)
	database.UnsubscribeByToken(sub.ConfirmToken)

	body := `{"email": "unsub@example.com", "list_id": "` + list.ID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// DNS check might fail
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}

	if w.Code == http.StatusOK {
		got, _ := database.GetSubscriber(sub.ID)
		if got.Status != "active" {
			t.Errorf("expected status 'active', got %q", got.Status)
		}
	}
}

func TestSubscribeWithListName(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("My Newsletter", "", "", "")

	body := `{"email": "test@example.com", "list": "My Newsletter"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// DNS check might fail
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Errorf("expected 201 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeWithInvalidListName(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"email": "test@example.com", "list": "Nonexistent"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Could be 400 from DNS or from list not found
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSubscribeNoLists(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 with no lists, got %d", w.Code)
	}
}

func TestSubscribeLongName(t *testing.T) {
	srv, database := testServer(t)
	database.CreateList("Test", "", "", "")

	longName := strings.Repeat("n", 300)
	body := `{"email": "test@example.com", "name": "` + longName + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should succeed (name is truncated) or fail DNS
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Errorf("expected 201 or 400, got %d", w.Code)
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
		"user@example",
	}
	for _, email := range invalid {
		if emailRegex.MatchString(email) {
			t.Errorf("expected %q to be invalid", email)
		}
	}
}

func TestHandlerReturnType(t *testing.T) {
	srv, _ := testServer(t)
	h := srv.Handler()
	if h == nil {
		t.Error("expected non-nil handler")
	}
}

func TestCORSOnSubscribe(t *testing.T) {
	srv, _ := testServer(t)

	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Subscribe endpoint has CORS middleware
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header on subscribe endpoint")
	}
}

func TestCORSOnAdminEndpoints(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS header on admin endpoint")
	}
}

// closedDBServer returns a server whose DB connection is already closed,
// to exercise error paths in handlers.
func closedDBServer(t *testing.T) *Server {
	t.Helper()
	f, _ := os.CreateTemp("", "inkdrift-api-closed-*.db")
	f.Close()
	t.Cleanup(func() { os.Remove(f.Name()) })

	database, _ := db.Open(f.Name())
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.CORS = "*"
	srv := NewServer(database, cfg)
	t.Cleanup(func() { srv.limiter.Close() })
	database.Close()
	return srv
}

func TestListListsDBError(t *testing.T) {
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestListCampaignsDBError(t *testing.T) {
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/campaigns", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestStatsDBError(t *testing.T) {
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestListSubscribersDBError(t *testing.T) {
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists/some-id/subscribers", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestSearchSubscribersDBError(t *testing.T) {
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/lists/some-id/subscribers/search?q=test", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestCreateListDBError(t *testing.T) {
	srv := closedDBServer(t)

	body := `{"name": "Test"}`
	req := httptest.NewRequest("POST", "/api/v1/lists", bytes.NewBufferString(body))
	req.Header.Set("X-API-Key", "test-api-key")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestSubscribeResubscribeWithSMTP(t *testing.T) {
	// With SMTP configured and domain set, resubscribe should go through pending path
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.CORS = "*"
	cfg.SMTP.Host = "smtp.example.com"
	cfg.SMTP.From = "test@example.com"
	cfg.Server.Domain = "newsletter.example.com"
	srv, database := testServerWithConfig(t, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriber("unsub@example.com", "", list.ID)
	database.UnsubscribeByToken(sub.ConfirmToken)

	body := `{"email": "unsub@example.com", "list_id": "` + list.ID + `"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// DNS may fail, but if it passes, should get 200 with pending message
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("expected 200 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}

	if w.Code == http.StatusOK {
		got, _ := database.GetSubscriber(sub.ID)
		if got.Status != "pending" {
			t.Errorf("expected status 'pending' with SMTP, got %q", got.Status)
		}
	}
}

func TestSubscribeDoubleOptIn(t *testing.T) {
	// With SMTP configured and domain, new subscribers go to pending
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.CORS = "*"
	cfg.SMTP.Host = "smtp.example.com"
	cfg.SMTP.From = "test@example.com"
	cfg.Server.Domain = "newsletter.example.com"
	srv, database := testServerWithConfig(t, cfg)

	database.CreateList("Test", "", "", "")

	body := `{"email": "new@example.com", "list": "Test"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// DNS may fail
	if w.Code != http.StatusCreated && w.Code != http.StatusBadRequest {
		t.Errorf("expected 201 or 400 (DNS), got %d: %s", w.Code, w.Body.String())
	}
}

func TestStatsCampaignsDBError(t *testing.T) {
	// Trigger the second DB error path in handleStats (ListCampaigns fails)
	// This is tricky to test without a partial closed DB, so we test the full error path
	srv := closedDBServer(t)

	req := httptest.NewRequest("GET", "/api/v1/stats", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSubscribeDBError(t *testing.T) {
	srv := closedDBServer(t)

	body := `{"email": "test@example.com"}`
	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	// Should get either 400 (DNS fail) or 500 (DB fail) — both are acceptable
	if w.Code != http.StatusBadRequest && w.Code != http.StatusInternalServerError {
		t.Errorf("expected 400 or 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAuthRateLimited(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.API.APIKey = "test-api-key"
	cfg.API.RateLimit = 10 // min enforced is 10
	srv, _ := testServerWithConfig(t, cfg)

	// Exhaust rate limit (10 requests)
	for i := 0; i < 12; i++ {
		req := httptest.NewRequest("GET", "/api/v1/lists", nil)
		req.Header.Set("X-API-Key", "wrong-key")
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/api/v1/lists", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "60" {
		t.Error("expected Retry-After header")
	}
}

func TestLoggedMiddleware(t *testing.T) {
	srv, _ := testServer(t)

	// Logged middleware is applied to all routes, just verify it doesn't break things
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSubscribeEmptyBody(t *testing.T) {
	srv, _ := testServer(t)

	req := httptest.NewRequest("POST", "/api/v1/subscribe", bytes.NewBufferString(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListSubscribersEmptyList(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Empty", "", "", "")

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
	if len(subs) != 0 {
		t.Errorf("expected 0 subscribers, got %d", len(subs))
	}
	if resp["total"].(float64) != 0 {
		t.Errorf("expected total 0, got %v", resp["total"])
	}
}

func TestSubscribePaginationBounds(t *testing.T) {
	srv, database := testServer(t)
	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)

	// Very large limit (should be capped at 1000)
	req := httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers?limit=5000", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Negative values should use defaults
	req = httptest.NewRequest("GET", "/api/v1/lists/"+list.ID+"/subscribers?limit=-1&offset=-5", nil)
	req.Header.Set("X-API-Key", "test-api-key")
	w = httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
