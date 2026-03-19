package smtp

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/google/uuid"
)

type Sender struct {
	cfg config.SMTPConfig
}

type Email struct {
	To        string
	Subject   string
	HTML      string
	Text      string
	Headers   map[string]string
	FromEmail string // per-email sender override (empty = use SMTP config)
	FromName  string // per-email sender name override (empty = use SMTP config)
}

func NewSender(cfg config.SMTPConfig) *Sender {
	return &Sender{cfg: cfg}
}

func (s *Sender) Send(email Email) error {
	if err := email.validate(); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	msg := s.buildMessage(email)
	auth := smtp.PlainAuth("", s.cfg.Username, s.cfg.Password, s.cfg.Host)

	if s.cfg.TLS {
		return s.sendTLS(addr, auth, email.To, msg)
	}
	return smtp.SendMail(addr, auth, s.cfg.From, []string{email.To}, msg)
}

func (e Email) validate() error {
	if e.To == "" {
		return fmt.Errorf("recipient is required")
	}
	if !strings.Contains(e.To, "@") || len(e.To) > 254 {
		return fmt.Errorf("invalid recipient address")
	}
	if e.Subject == "" {
		return fmt.Errorf("subject is required")
	}
	if e.HTML == "" && e.Text == "" {
		return fmt.Errorf("email body is required")
	}
	return nil
}

// sanitizeHeaderValue strips CR/LF to prevent SMTP header injection
func sanitizeHeaderValue(v string) string {
	v = strings.ReplaceAll(v, "\r", "")
	v = strings.ReplaceAll(v, "\n", "")
	return v
}

func (s *Sender) sendTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid SMTP address: %w", err)
	}

	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}

	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 10 * time.Second}, "tcp", addr, tlsConfig)
	if err != nil {
		// Fall back to STARTTLS
		return s.sendSTARTTLS(addr, auth, to, msg)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Close()

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing email: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing email: %w", err)
	}

	return client.Quit()
}

func (s *Sender) sendSTARTTLS(addr string, auth smtp.Auth, to string, msg []byte) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid SMTP address: %w", err)
	}

	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("connecting to SMTP: %w", err)
	}
	defer conn.Close()

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("creating SMTP client: %w", err)
	}
	defer client.Close()

	tlsConfig := &tls.Config{
		ServerName: host,
		MinVersion: tls.VersionTLS12,
	}
	if err := client.StartTLS(tlsConfig); err != nil {
		return fmt.Errorf("STARTTLS: %w", err)
	}

	if err := client.Auth(auth); err != nil {
		return fmt.Errorf("SMTP auth: %w", err)
	}
	if err := client.Mail(s.cfg.From); err != nil {
		return fmt.Errorf("SMTP MAIL FROM: %w", err)
	}
	if err := client.Rcpt(to); err != nil {
		return fmt.Errorf("SMTP RCPT TO: %w", err)
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA: %w", err)
	}
	if _, err := w.Write(msg); err != nil {
		return fmt.Errorf("writing email: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("closing email: %w", err)
	}

	return client.Quit()
}

func (s *Sender) buildMessage(email Email) []byte {
	var b strings.Builder

	// Per-email overrides for multi-tenant sending
	fromAddr := s.cfg.From
	if email.FromEmail != "" {
		fromAddr = email.FromEmail
	}
	fromName := sanitizeHeaderValue(s.cfg.FromName)
	if email.FromName != "" {
		fromName = sanitizeHeaderValue(email.FromName)
	}
	if fromName == "" {
		fromName = fromAddr
	}

	// Generate unique Message-ID for deliverability
	domain := s.cfg.Host
	if parts := strings.SplitN(fromAddr, "@", 2); len(parts) == 2 {
		domain = parts[1]
	}
	messageID := fmt.Sprintf("<%s@%s>", uuid.New().String(), domain)

	// RFC 5322: quote display name to prevent display-name spoofing with angle brackets
	quotedName := `"` + strings.ReplaceAll(fromName, `"`, `\"`) + `"`
	b.WriteString(fmt.Sprintf("From: %s <%s>\r\n", quotedName, sanitizeHeaderValue(fromAddr)))
	b.WriteString(fmt.Sprintf("To: %s\r\n", sanitizeHeaderValue(email.To)))
	b.WriteString(fmt.Sprintf("Subject: %s\r\n", sanitizeHeaderValue(email.Subject)))
	b.WriteString(fmt.Sprintf("Date: %s\r\n", time.Now().Format(time.RFC1123Z)))
	b.WriteString(fmt.Sprintf("Message-ID: %s\r\n", messageID))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Precedence: bulk\r\n")
	b.WriteString(fmt.Sprintf("Return-Path: <%s>\r\n", sanitizeHeaderValue(s.cfg.From)))

	// Sanitize custom headers to prevent header injection
	for k, v := range email.Headers {
		k = sanitizeHeaderValue(k)
		v = sanitizeHeaderValue(v)
		if k == "" {
			continue
		}
		b.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}

	if email.HTML != "" && email.Text != "" {
		boundary := fmt.Sprintf("inkdrift-%s", uuid.New().String())
		b.WriteString(fmt.Sprintf("Content-Type: multipart/alternative; boundary=\"%s\"\r\n", boundary))
		b.WriteString("\r\n")
		b.WriteString(fmt.Sprintf("--%s\r\n", boundary))
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(email.Text)
		b.WriteString(fmt.Sprintf("\r\n--%s\r\n", boundary))
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(email.HTML)
		b.WriteString(fmt.Sprintf("\r\n--%s--\r\n", boundary))
	} else if email.HTML != "" {
		b.WriteString("Content-Type: text/html; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(email.HTML)
	} else {
		b.WriteString("Content-Type: text/plain; charset=\"utf-8\"\r\n")
		b.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		b.WriteString(email.Text)
	}

	return []byte(b.String())
}

func (s *Sender) TestConnection() error {
	addr := net.JoinHostPort(s.cfg.Host, fmt.Sprintf("%d", s.cfg.Port))
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return fmt.Errorf("cannot connect to %s: %w", addr, err)
	}
	conn.Close()
	return nil
}
