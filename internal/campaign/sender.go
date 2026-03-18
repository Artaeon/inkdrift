package campaign

import (
	"fmt"
	"html/template"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
	"github.com/artaeon/inkdrift/internal/render"
	"github.com/artaeon/inkdrift/internal/smtp"
)

type Sender struct {
	db     *db.DB
	smtp   *smtp.Sender
	cfg    *config.Config
	onSend func(email string, err error) // progress callback
}

func NewSender(database *db.DB, smtpSender *smtp.Sender, cfg *config.Config) *Sender {
	return &Sender{
		db:   database,
		smtp: smtpSender,
		cfg:  cfg,
	}
}

func (s *Sender) OnSend(fn func(email string, err error)) {
	s.onSend = fn
}

func (s *Sender) Send(campaignID string) error {
	campaign, err := s.db.GetCampaign(campaignID)
	if err != nil {
		return fmt.Errorf("getting campaign: %w", err)
	}

	if campaign.Status != "draft" {
		return fmt.Errorf("campaign is %s, not draft", campaign.Status)
	}

	subscribers, err := s.db.GetActiveSubscribers(campaign.ListID)
	if err != nil {
		return fmt.Errorf("getting subscribers: %w", err)
	}

	if len(subscribers) == 0 {
		return fmt.Errorf("no active subscribers in list")
	}

	list, err := s.db.GetList(campaign.ListID)
	if err != nil {
		return fmt.Errorf("getting list: %w", err)
	}

	if err := s.db.UpdateCampaignStatus(campaignID, "sending"); err != nil {
		return fmt.Errorf("updating campaign status: %w", err)
	}

	// Determine the template body
	tmplBody := campaign.Body
	if campaign.TemplateID != "" {
		tmpl, err := s.db.GetTemplate(campaign.TemplateID)
		if err == nil {
			tmplBody = tmpl.Body
		}
	}

	sentCount := 0
	failedCount := 0

	for _, sub := range subscribers {
		ctx := render.Context{
			SubscriberName:  sub.Name,
			SubscriberEmail: sub.Email,
			UnsubscribeURL:  s.unsubscribeURL(sub.ConfirmToken),
			ListName:        list.Name,
			SenderName:      s.cfg.SMTP.FromName,
			Content:         template.HTML(campaign.Body),
			Year:            time.Now().Year(),
		}

		html, err := render.RenderHTML(tmplBody, ctx)
		if err != nil {
			failedCount++
			s.db.LogSend(campaignID, sub.ID, "failed", err.Error())
			if s.onSend != nil {
				s.onSend(sub.Email, err)
			}
			continue
		}

		text := render.RenderText(html)

		email := smtp.Email{
			To:      sub.Email,
			Subject: campaign.Subject,
			HTML:    html,
			Text:    text,
			Headers: map[string]string{
				"List-Unsubscribe":      fmt.Sprintf("<%s>", s.unsubscribeURL(sub.ConfirmToken)),
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
				"X-Mailer":             "InkDrift",
			},
		}

		if err := s.smtp.Send(email); err != nil {
			failedCount++
			s.db.LogSend(campaignID, sub.ID, "failed", err.Error())
			if s.onSend != nil {
				s.onSend(sub.Email, err)
			}
		} else {
			sentCount++
			s.db.LogSend(campaignID, sub.ID, "sent", "")
			if s.onSend != nil {
				s.onSend(sub.Email, nil)
			}
		}

		// Rate limit: small delay between sends to avoid SMTP throttling
		time.Sleep(100 * time.Millisecond)
	}

	status := "sent"
	if sentCount == 0 {
		status = "failed"
	}

	s.db.UpdateCampaignStatus(campaignID, status)
	s.db.UpdateCampaignStats(campaignID, sentCount, failedCount)

	return nil
}

func (s *Sender) unsubscribeURL(token string) string {
	domain := s.cfg.Server.Domain
	if domain == "" {
		domain = fmt.Sprintf("localhost:%d", s.cfg.API.Port)
	}
	scheme := "https"
	if domain == fmt.Sprintf("localhost:%d", s.cfg.API.Port) {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s/api/v1/unsubscribe?token=%s", scheme, domain, token)
}
