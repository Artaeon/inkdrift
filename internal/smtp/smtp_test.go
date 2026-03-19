package smtp

import (
	"strings"
	"testing"

	"github.com/artaeon/inkdrift/internal/config"
)

func TestEmailValidate(t *testing.T) {
	tests := []struct {
		name    string
		email   Email
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid HTML email",
			email:   Email{To: "test@example.com", Subject: "Hello", HTML: "<p>Hi</p>"},
			wantErr: false,
		},
		{
			name:    "valid text email",
			email:   Email{To: "test@example.com", Subject: "Hello", Text: "Hi"},
			wantErr: false,
		},
		{
			name:    "valid multipart email",
			email:   Email{To: "test@example.com", Subject: "Hello", HTML: "<p>Hi</p>", Text: "Hi"},
			wantErr: false,
		},
		{
			name:    "missing recipient",
			email:   Email{To: "", Subject: "Hello", HTML: "<p>Hi</p>"},
			wantErr: true,
			errMsg:  "recipient is required",
		},
		{
			name:    "invalid recipient no @",
			email:   Email{To: "invalid", Subject: "Hello", HTML: "<p>Hi</p>"},
			wantErr: true,
			errMsg:  "invalid recipient",
		},
		{
			name:    "recipient too long",
			email:   Email{To: strings.Repeat("a", 250) + "@b.com", Subject: "Hello", HTML: "<p>Hi</p>"},
			wantErr: true,
			errMsg:  "invalid recipient",
		},
		{
			name:    "missing subject",
			email:   Email{To: "test@example.com", Subject: "", HTML: "<p>Hi</p>"},
			wantErr: true,
			errMsg:  "subject is required",
		},
		{
			name:    "missing body",
			email:   Email{To: "test@example.com", Subject: "Hello", HTML: "", Text: ""},
			wantErr: true,
			errMsg:  "body is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.email.validate()
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				} else if tt.errMsg != "" && !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("expected error containing %q, got %q", tt.errMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestSanitizeHeaderValue(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal value", "normal value"},
		{"inject\r\nBCC: evil@attacker.com", "injectBCC: evil@attacker.com"},
		{"inject\nBCC: evil@attacker.com", "injectBCC: evil@attacker.com"},
		{"inject\rBCC: evil@attacker.com", "injectBCC: evil@attacker.com"},
		{"", ""},
		{"no special chars", "no special chars"},
		{"\r\n\r\n", ""},
	}

	for _, tt := range tests {
		got := sanitizeHeaderValue(tt.input)
		if got != tt.expected {
			t.Errorf("sanitizeHeaderValue(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestBuildMessageHTMLOnly(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		From:     "sender@example.com",
		FromName: "Sender Name",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test Subject",
		HTML:    "<p>Hello</p>",
	}

	msg := string(s.buildMessage(email))

	if !strings.Contains(msg, "From: Sender Name <sender@example.com>") {
		t.Error("missing From header")
	}
	if !strings.Contains(msg, "To: recipient@example.com") {
		t.Error("missing To header")
	}
	if !strings.Contains(msg, "Subject: Test Subject") {
		t.Error("missing Subject header")
	}
	if !strings.Contains(msg, "MIME-Version: 1.0") {
		t.Error("missing MIME-Version header")
	}
	if !strings.Contains(msg, "Content-Type: text/html") {
		t.Error("expected HTML content type")
	}
	if !strings.Contains(msg, "Message-ID:") {
		t.Error("missing Message-ID header")
	}
	if !strings.Contains(msg, "Precedence: bulk") {
		t.Error("missing Precedence header")
	}
	if !strings.Contains(msg, "Return-Path:") {
		t.Error("missing Return-Path header")
	}
	if !strings.Contains(msg, "<p>Hello</p>") {
		t.Error("missing body content")
	}
}

func TestBuildMessageTextOnly(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		Text:    "Plain text",
	}

	msg := string(s.buildMessage(email))

	if !strings.Contains(msg, "Content-Type: text/plain") {
		t.Error("expected plain text content type")
	}
	if !strings.Contains(msg, "Plain text") {
		t.Error("missing body content")
	}
}

func TestBuildMessageMultipart(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		HTML:    "<p>HTML</p>",
		Text:    "Plain",
	}

	msg := string(s.buildMessage(email))

	if !strings.Contains(msg, "multipart/alternative") {
		t.Error("expected multipart content type")
	}
	if !strings.Contains(msg, "text/plain") {
		t.Error("missing plain text part")
	}
	if !strings.Contains(msg, "text/html") {
		t.Error("missing HTML part")
	}
	if !strings.Contains(msg, "Plain") {
		t.Error("missing text content")
	}
	if !strings.Contains(msg, "<p>HTML</p>") {
		t.Error("missing HTML content")
	}
}

func TestBuildMessageCustomHeaders(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
		Headers: map[string]string{
			"List-Unsubscribe": "<https://example.com/unsub>",
			"X-Mailer":         "InkDrift",
		},
	}

	msg := string(s.buildMessage(email))

	if !strings.Contains(msg, "List-Unsubscribe: <https://example.com/unsub>") {
		t.Error("missing custom List-Unsubscribe header")
	}
	if !strings.Contains(msg, "X-Mailer: InkDrift") {
		t.Error("missing custom X-Mailer header")
	}
}

func TestBuildMessageHeaderInjectionPrevention(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test\r\nBCC: evil@attacker.com",
		HTML:    "<p>Hi</p>",
	}

	msg := string(s.buildMessage(email))

	// Subject should be sanitized - no newlines means no separate BCC header line
	lines := strings.Split(msg, "\r\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "BCC:") {
			t.Error("header injection not prevented in subject")
		}
	}
	// The sanitized subject should be on one line
	if !strings.Contains(msg, "Subject: TestBCC: evil@attacker.com\r\n") {
		t.Error("subject not properly sanitized")
	}
}

func TestBuildMessageEmptyHeaderKeySkipped(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
		Headers: map[string]string{
			"":     "should be skipped",
			"\r\n": "should also be skipped after sanitize",
		},
	}

	msg := string(s.buildMessage(email))
	// Empty key headers should be skipped
	if strings.Contains(msg, "should be skipped") {
		t.Error("empty key header should have been skipped")
	}
}

func TestBuildMessageFromNameFallback(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
		// FromName is empty
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
	}

	msg := string(s.buildMessage(email))
	// When FromName is empty, should use From address as display name
	if !strings.Contains(msg, "From: sender@example.com <sender@example.com>") {
		t.Errorf("expected From address as fallback name, got message: %s", msg[:200])
	}
}

func TestBuildMessageDomainExtraction(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@newsletter.example.com",
	})

	email := Email{
		To:      "recipient@example.com",
		Subject: "Test",
		HTML:    "<p>Hi</p>",
	}

	msg := string(s.buildMessage(email))
	// Message-ID should use domain from From address, not SMTP host
	if !strings.Contains(msg, "@newsletter.example.com>") {
		t.Error("Message-ID should use domain from From address")
	}
}

func TestSendValidationError(t *testing.T) {
	s := NewSender(config.SMTPConfig{
		Host: "smtp.example.com",
		From: "sender@example.com",
	})

	// Missing subject should fail validation before connecting
	err := s.Send(Email{To: "test@example.com", HTML: "<p>Hi</p>"})
	if err == nil {
		t.Error("expected validation error")
	}
	if !strings.Contains(err.Error(), "subject is required") {
		t.Errorf("expected subject validation error, got: %v", err)
	}
}

func TestNewSender(t *testing.T) {
	cfg := config.SMTPConfig{Host: "smtp.example.com", Port: 587}
	s := NewSender(cfg)
	if s == nil {
		t.Error("expected non-nil sender")
	}
	if s.cfg.Host != "smtp.example.com" {
		t.Errorf("expected host 'smtp.example.com', got %q", s.cfg.Host)
	}
}

func TestBuildMessageFromOverride(t *testing.T) {
	cfg := config.SMTPConfig{
		Host:     "smtp.example.com",
		From:     "global@example.com",
		FromName: "Global Name",
	}
	s := NewSender(cfg)

	email := Email{
		To:        "user@test.com",
		Subject:   "Hello",
		HTML:      "<p>Hi</p>",
		FromEmail: "local@site-a.com",
		FromName:  "Site A",
	}

	msg := string(s.buildMessage(email))

	// From header should use the override, not global
	if !strings.Contains(msg, "Site A <local@site-a.com>") {
		t.Error("expected per-email From override in message headers")
	}
	// From: line should not contain global address
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(line, "From:") && strings.Contains(line, "global@example.com") {
			t.Error("From: header should use override, not global")
		}
	}
	// Message-ID domain should use the override email's domain
	if !strings.Contains(msg, "@site-a.com>") {
		t.Error("expected Message-ID domain from override email")
	}
}

func TestBuildMessageFromOverridePartial(t *testing.T) {
	cfg := config.SMTPConfig{
		Host:     "smtp.example.com",
		From:     "global@example.com",
		FromName: "Global Name",
	}
	s := NewSender(cfg)

	// Only override email, not name — should use global name
	email := Email{
		To:        "user@test.com",
		Subject:   "Hello",
		HTML:      "<p>Hi</p>",
		FromEmail: "local@site-a.com",
	}

	msg := string(s.buildMessage(email))
	if !strings.Contains(msg, "Global Name <local@site-a.com>") {
		t.Errorf("expected global name with override email, got:\n%s", msg)
	}
}

func TestBuildMessageNoOverride(t *testing.T) {
	cfg := config.SMTPConfig{
		Host:     "smtp.example.com",
		From:     "global@example.com",
		FromName: "Global Name",
	}
	s := NewSender(cfg)

	email := Email{
		To:      "user@test.com",
		Subject: "Hello",
		HTML:    "<p>Hi</p>",
	}

	msg := string(s.buildMessage(email))
	if !strings.Contains(msg, "Global Name <global@example.com>") {
		t.Error("expected global From when no override")
	}
}
