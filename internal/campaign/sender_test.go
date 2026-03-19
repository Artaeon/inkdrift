package campaign

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
	"github.com/artaeon/inkdrift/internal/smtp"
)

// mockSMTP records sent emails and can simulate failures.
type mockSMTP struct {
	sent    []smtp.Email
	failFor map[string]error // email address -> error
}

func (m *mockSMTP) Send(email smtp.Email) error {
	m.sent = append(m.sent, email)
	if err, ok := m.failFor[email.To]; ok {
		return err
	}
	return nil
}

func testDB(t *testing.T) *db.DB {
	t.Helper()
	f, err := os.CreateTemp("", "inkdrift-campaign-test-*.db")
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
	return database
}

func testCfg() *config.Config {
	cfg := config.DefaultConfig()
	cfg.SMTP.Host = "smtp.example.com"
	cfg.SMTP.Port = 587
	cfg.SMTP.From = "test@example.com"
	cfg.SMTP.FromName = "Test Sender"
	cfg.Server.Domain = "example.com"
	return cfg
}

func TestNewSender(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)
	if s == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestOnSend(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	called := false
	s.OnSend(func(email string, idx, total int, err error) {
		called = true
	})

	if s.onSend == nil {
		t.Error("expected onSend to be set")
	}
	s.onSend("test@example.com", 1, 1, nil)
	if !called {
		t.Error("expected callback to be called")
	}
}

func TestSendSuccess(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "Alice", list.ID)
	database.AddSubscriber("b@example.com", "Bob", list.ID)
	c, _ := database.CreateCampaign("Test", "Subject", "<p>Hello</p>", list.ID)

	var progress []string
	s.OnSend(func(email string, idx, total int, err error) {
		progress = append(progress, email)
	})

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	if len(mock.sent) != 2 {
		t.Errorf("expected 2 emails sent, got %d", len(mock.sent))
	}
	if len(progress) != 2 {
		t.Errorf("expected 2 progress callbacks, got %d", len(progress))
	}

	// Check campaign status
	updated, _ := database.GetCampaign(c.ID)
	if updated.Status != "sent" {
		t.Errorf("expected status 'sent', got %q", updated.Status)
	}
	if updated.SentCount != 2 {
		t.Errorf("expected sent_count 2, got %d", updated.SentCount)
	}
}

func TestSendPartialFailure(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"fail@example.com": fmt.Errorf("SMTP error"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("ok@example.com", "", list.ID)
	database.AddSubscriber("fail@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	updated, _ := database.GetCampaign(c.ID)
	if updated.Status != "partial" {
		t.Errorf("expected status 'partial', got %q", updated.Status)
	}
	if updated.SentCount != 1 {
		t.Errorf("expected sent_count 1, got %d", updated.SentCount)
	}
	if updated.FailedCount != 1 {
		t.Errorf("expected failed_count 1, got %d", updated.FailedCount)
	}
}

func TestSendAllFail(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"a@example.com": fmt.Errorf("SMTP error"),
			"b@example.com": fmt.Errorf("SMTP error"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	updated, _ := database.GetCampaign(c.ID)
	if updated.Status != "failed" {
		t.Errorf("expected status 'failed', got %q", updated.Status)
	}
}

func TestSendBounceHandling(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"bounce@example.com": fmt.Errorf("550 User unknown"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("ok@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("bounce@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	// Bounced subscriber should be marked as bounced
	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 550 error, got %q", updated.Status)
	}
}

func TestSendNonDraftCampaign(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "Body", list.ID)
	database.ClaimCampaignForSending(c.ID)

	err := s.Send(c.ID)
	if err == nil {
		t.Error("expected error for non-draft campaign")
	}
}

func TestSendNoSubscribers(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Empty List", "", "", "")
	c, _ := database.CreateCampaign("Test", "Sub", "Body", list.ID)

	err := s.Send(c.ID)
	if err == nil {
		t.Error("expected error for empty subscriber list")
	}
}

func TestSendCampaignNotFound(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	err := s.Send("nonexistent-id")
	if err == nil {
		t.Error("expected error for nonexistent campaign")
	}
}

func TestResendFailedWrongStatus(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "Body", list.ID)
	err := s.ResendFailed(c.ID)
	if err == nil {
		t.Error("expected error: retry only for partial/failed/sending")
	}
}

func TestResendFailedSuccess(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	sub1, _ := database.AddSubscriber("sent@example.com", "", list.ID)
	database.AddSubscriber("retry@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	// Simulate first send: sub1 succeeded, sub2 failed
	database.ClaimCampaignForSending(c.ID)
	database.LogSend(c.ID, sub1.ID, "sent", "")
	database.UpdateCampaignStatus(c.ID, "partial")
	database.UpdateCampaignStats(c.ID, 1, 1)

	if err := s.ResendFailed(c.ID); err != nil {
		t.Fatal(err)
	}

	// Only retry@example.com should have been sent
	if len(mock.sent) != 1 {
		t.Errorf("expected 1 retry email, got %d", len(mock.sent))
	}
	if mock.sent[0].To != "retry@example.com" {
		t.Errorf("expected retry for 'retry@example.com', got %q", mock.sent[0].To)
	}

	// Check updated stats - sentCount should accumulate from previous
	updated, _ := database.GetCampaign(c.ID)
	if updated.SentCount != 2 {
		t.Errorf("expected sent_count 2 (1 prev + 1 retry), got %d", updated.SentCount)
	}
}

func TestResendFailedAllSent(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "Body", list.ID)
	database.ClaimCampaignForSending(c.ID)
	database.LogSend(c.ID, sub.ID, "sent", "")
	database.UpdateCampaignStatus(c.ID, "partial")

	err := s.ResendFailed(c.ID)
	if err == nil {
		t.Error("expected error: all subscribers already sent")
	}
}

func TestSendWithTemplate(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "Alice", list.ID)
	tmpl, _ := database.CreateTemplate("Wrapper", "<html><body>{{.Content}}</body></html>")

	c, _ := database.CreateCampaign("Test", "Sub", "<p>Hello</p>", list.ID)
	database.SetCampaignTemplate(c.ID, tmpl.ID)

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
	// HTML should be wrapped in template
	if !strings.Contains(mock.sent[0].HTML, "<html>") {
		t.Error("expected template wrapper in HTML")
	}
	if !strings.Contains(mock.sent[0].HTML, "<p>Hello</p>") {
		t.Error("expected campaign body in HTML")
	}
}

func TestSendEmailHeaders(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	sub, _ := database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Subject Line", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email, got %d", len(mock.sent))
	}
	email := mock.sent[0]
	if email.Subject != "Subject Line" {
		t.Errorf("expected subject 'Subject Line', got %q", email.Subject)
	}
	if email.To != "a@example.com" {
		t.Errorf("expected to 'a@example.com', got %q", email.To)
	}
	// Check List-Unsubscribe header
	unsubHeader := email.Headers["List-Unsubscribe"]
	if unsubHeader == "" {
		t.Error("missing List-Unsubscribe header")
	}
	if !strings.Contains(unsubHeader, sub.UnsubscribeToken) {
		t.Error("List-Unsubscribe should contain subscriber unsubscribe token")
	}
	// Check X-Mailer
	if email.Headers["X-Mailer"] != "InkDrift" {
		t.Errorf("expected X-Mailer 'InkDrift', got %q", email.Headers["X-Mailer"])
	}
	// Should have text version
	if email.Text == "" {
		t.Error("expected text fallback")
	}
}

func TestSendTemplateRenderError(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)

	// Invalid Go template syntax in body
	c, _ := database.CreateCampaign("Test", "Sub", "{{.Invalid", list.ID)

	// Should not return error (logs it), but should mark as failed
	s.Send(c.ID)

	updated, _ := database.GetCampaign(c.ID)
	if updated.Status != "failed" {
		t.Errorf("expected status 'failed' for render error, got %q", updated.Status)
	}
}

func TestUnsubscribeURL(t *testing.T) {
	database := testDB(t)

	tests := []struct {
		name      string
		domain    string
		apiPort   int
		wantHTTPS bool
		wantHost  string
	}{
		{
			name:      "with domain",
			domain:    "newsletter.example.com",
			apiPort:   3377,
			wantHTTPS: true,
			wantHost:  "newsletter.example.com",
		},
		{
			name:      "without domain (localhost)",
			domain:    "",
			apiPort:   3377,
			wantHTTPS: false,
			wantHost:  "localhost:3377",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testCfg()
			cfg.Server.Domain = tt.domain
			cfg.API.Port = tt.apiPort
			s := NewSender(database, &mockSMTP{}, cfg)

			url := s.unsubscribeURL("test-token")
			if tt.wantHTTPS {
				if url[:8] != "https://" {
					t.Errorf("expected https, got %s", url)
				}
			} else {
				if url[:7] != "http://" {
					t.Errorf("expected http, got %s", url)
				}
			}
			if !strings.Contains(url, tt.wantHost) {
				t.Errorf("expected host %q in URL %q", tt.wantHost, url)
			}
			if !strings.Contains(url, "test-token") {
				t.Errorf("expected token in URL %q", url)
			}
		})
	}
}

func TestSendDeletedList(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	c, _ := database.CreateCampaign("Test", "Sub", "Body", list.ID)
	database.DeleteList(list.ID)

	err := s.Send(c.ID)
	if err == nil {
		t.Error("expected error for deleted list")
	}
}

func TestSendWithListFromIdentity(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	// Create list with per-list sender identity
	list, _ := database.CreateList("Site A", "", "news@site-a.com", "Site A News")
	database.AddSubscriber("user@example.com", "User", list.ID)
	c, _ := database.CreateCampaign("Welcome", "Hello", "<p>Hi</p>", list.ID)

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.sent))
	}
	if mock.sent[0].FromEmail != "news@site-a.com" {
		t.Errorf("expected from_email 'news@site-a.com', got %q", mock.sent[0].FromEmail)
	}
	if mock.sent[0].FromName != "Site A News" {
		t.Errorf("expected from_name 'Site A News', got %q", mock.sent[0].FromName)
	}
}

func TestSendBounce551(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("551 User not local"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 551 error, got %q", updated.Status)
	}
}

func TestSendBounce552(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("552 Mailbox full"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 552 error, got %q", updated.Status)
	}
}

func TestSendBounce553(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("553 Requested action not taken"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 553 error, got %q", updated.Status)
	}
}

func TestSendBounce554(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("554 Transaction failed"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 554 error, got %q", updated.Status)
	}
}

func TestSendBounceMailboxNotFound(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("mailbox not found"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 'mailbox not found' error, got %q", updated.Status)
	}
}

func TestSendBounceDoesNotExist(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("does not exist"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	bounceSub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	updated, _ := database.GetSubscriber(bounceSub.ID)
	if updated.Status != "bounced" {
		t.Errorf("expected status 'bounced' for 'does not exist' error, got %q", updated.Status)
	}
}

func TestSendNonBounceError(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"b@example.com": fmt.Errorf("421 try again later"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	sub, _ := database.AddSubscriber("b@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	s.Send(c.ID)

	// Non-bounce errors should NOT mark as bounced
	updated, _ := database.GetSubscriber(sub.ID)
	if updated.Status != "active" {
		t.Errorf("expected status 'active' for non-bounce error, got %q", updated.Status)
	}
}

func TestSendTemplateRenderErrorWithOnSend(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)

	// Invalid Go template syntax
	c, _ := database.CreateCampaign("Test", "Sub", "{{.Invalid", list.ID)

	var errors []error
	s.OnSend(func(email string, idx, total int, err error) {
		errors = append(errors, err)
	})

	s.Send(c.ID)

	if len(errors) != 1 {
		t.Fatalf("expected 1 error callback, got %d", len(errors))
	}
	if errors[0] == nil {
		t.Error("expected non-nil error for render failure")
	}
}

func TestSendSMTPFailWithOnSend(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{
		failFor: map[string]error{
			"a@example.com": fmt.Errorf("SMTP connection failed"),
		},
	}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	var errors []error
	s.OnSend(func(email string, idx, total int, err error) {
		errors = append(errors, err)
	})

	s.Send(c.ID)

	if len(errors) != 1 {
		t.Fatalf("expected 1 callback, got %d", len(errors))
	}
	if errors[0] == nil {
		t.Error("expected non-nil error for SMTP failure")
	}
}

func TestResendFailedFromFailedStatus(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	// Put campaign into "failed" status
	database.ClaimCampaignForSending(c.ID)
	database.UpdateCampaignStatus(c.ID, "failed")

	if err := s.ResendFailed(c.ID); err != nil {
		t.Fatalf("expected retry from failed status to succeed: %v", err)
	}
}

func TestResendFailedFromSendingStatus(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	// Put campaign into "sending" status (stuck)
	database.ClaimCampaignForSending(c.ID)

	if err := s.ResendFailed(c.ID); err != nil {
		t.Fatalf("expected retry from sending status to succeed: %v", err)
	}
}

func TestResendFailedFromSentStatus(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	s := NewSender(database, &mockSMTP{}, cfg)

	list, _ := database.CreateList("Test", "", "", "")
	database.AddSubscriber("a@example.com", "", list.ID)
	c, _ := database.CreateCampaign("Test", "Sub", "<p>Body</p>", list.ID)

	database.ClaimCampaignForSending(c.ID)
	database.UpdateCampaignStatus(c.ID, "sent")

	err := s.ResendFailed(c.ID)
	if err == nil {
		t.Error("expected error: retry not available for sent campaigns")
	}
	if !strings.Contains(err.Error(), "sent") {
		t.Errorf("expected error mentioning 'sent', got: %v", err)
	}
}

func TestSendWithGlobalFallback(t *testing.T) {
	database := testDB(t)
	cfg := testCfg()
	mock := &mockSMTP{}
	s := NewSender(database, mock, cfg)

	// Create list WITHOUT per-list sender — should use empty (global fallback)
	list, _ := database.CreateList("Global", "", "", "")
	database.AddSubscriber("user@example.com", "User", list.ID)
	c, _ := database.CreateCampaign("News", "Update", "<p>Hi</p>", list.ID)

	if err := s.Send(c.ID); err != nil {
		t.Fatal(err)
	}

	if len(mock.sent) != 1 {
		t.Fatalf("expected 1 email sent, got %d", len(mock.sent))
	}
	// Empty = global SMTP config handles it
	if mock.sent[0].FromEmail != "" {
		t.Errorf("expected empty from_email for global fallback, got %q", mock.sent[0].FromEmail)
	}
}
