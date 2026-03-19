package campaign

import (
	"fmt"
	"html/template"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/artaeon/inkdrift/internal/config"
	"github.com/artaeon/inkdrift/internal/db"
	"github.com/artaeon/inkdrift/internal/render"
	"github.com/artaeon/inkdrift/internal/smtp"
)

// EmailSender abstracts email sending for testability.
type EmailSender interface {
	Send(email smtp.Email) error
}

type Sender struct {
	db     *db.DB
	smtp   EmailSender
	cfg    *config.Config
	onSend func(email string, idx, total int, err error) // progress callback
}

func NewSender(database *db.DB, smtpSender EmailSender, cfg *config.Config) *Sender {
	return &Sender{
		db:   database,
		smtp: smtpSender,
		cfg:  cfg,
	}
}

func (s *Sender) OnSend(fn func(email string, idx, total int, err error)) {
	s.onSend = fn
}

func (s *Sender) Send(campaignID string) error {
	return s.sendToSubscribers(campaignID, false)
}

// ResendFailed retries sending only to subscribers that failed in a previous send.
func (s *Sender) ResendFailed(campaignID string) error {
	return s.sendToSubscribers(campaignID, true)
}

func (s *Sender) sendToSubscribers(campaignID string, retryOnly bool) error {
	campaign, err := s.db.GetCampaign(campaignID)
	if err != nil {
		return fmt.Errorf("getting campaign: %w", err)
	}

	if retryOnly {
		// Allow retry for partial/failed/sending campaigns
		if campaign.Status != "partial" && campaign.Status != "failed" && campaign.Status != "sending" {
			return fmt.Errorf("campaign is %s — retry is only available for partial, failed, or stuck-sending campaigns", campaign.Status)
		}
	} else {
		if campaign.Status != "draft" {
			return fmt.Errorf("campaign is %s, not draft", campaign.Status)
		}
	}

	list, err := s.db.GetList(campaign.ListID)
	if err != nil {
		return fmt.Errorf("list not found (may have been deleted): %w", err)
	}

	subscribers, err := s.db.GetActiveSubscribers(campaign.ListID)
	if err != nil {
		return fmt.Errorf("getting subscribers: %w", err)
	}

	if len(subscribers) == 0 {
		return fmt.Errorf("no active subscribers in list '%s'", list.Name)
	}

	// For retry: filter to only subscribers that failed previously
	if retryOnly {
		alreadySent, err := s.db.GetSentSubscriberIDs(campaignID)
		if err != nil {
			return fmt.Errorf("checking send log: %w", err)
		}
		var retry []db.Subscriber
		for _, sub := range subscribers {
			if !alreadySent[sub.ID] {
				retry = append(retry, sub)
			}
		}
		subscribers = retry
		if len(subscribers) == 0 {
			return fmt.Errorf("all subscribers already received this campaign")
		}
	}

	// Claim campaign for sending (atomically transition from draft)
	if !retryOnly {
		if err := s.db.ClaimCampaignForSending(campaignID); err != nil {
			return fmt.Errorf("cannot send: %w", err)
		}
	} else {
		// For retry, just mark as sending again
		if err := s.db.UpdateCampaignStatus(campaignID, "sending"); err != nil {
			return fmt.Errorf("updating status: %w", err)
		}
	}

	// Determine the template body
	tmplBody := campaign.Body
	if campaign.TemplateID != "" {
		tmpl, err := s.db.GetTemplate(campaign.TemplateID)
		if err == nil {
			tmplBody = tmpl.Body
		}
	}

	sentCount := campaign.SentCount
	failedCount := 0
	total := len(subscribers)

	// Per-list sender identity (falls back to global SMTP config)
	listFromEmail := list.FromEmail
	listFromName := list.FromName
	senderName := s.cfg.SMTP.FromName
	if listFromName != "" {
		senderName = listFromName
	}

	for i, sub := range subscribers {
		ctx := render.Context{
			SubscriberName:  sub.Name,
			SubscriberEmail: sub.Email,
			UnsubscribeURL:  s.unsubscribeURL(sub.ConfirmToken),
			ListName:        list.Name,
			SenderName:      senderName,
			Content:         template.HTML(campaign.Body),
			Year:            time.Now().Year(),
		}

		html, err := render.RenderHTML(tmplBody, ctx)
		if err != nil {
			failedCount++
			if logErr := s.db.LogSend(campaignID, sub.ID, "failed", err.Error()); logErr != nil {
				log.Printf("failed to log send: %v", logErr)
			}
			if s.onSend != nil {
				s.onSend(sub.Email, i+1, total, err)
			}
			continue
		}

		text := render.RenderText(html)

		email := smtp.Email{
			To:        sub.Email,
			Subject:   campaign.Subject,
			HTML:      html,
			Text:      text,
			FromEmail: listFromEmail,
			FromName:  listFromName,
			Headers: map[string]string{
				"List-Unsubscribe":      fmt.Sprintf("<%s>", s.unsubscribeURL(sub.ConfirmToken)),
				"List-Unsubscribe-Post": "List-Unsubscribe=One-Click",
				"X-Mailer":             "InkDrift",
			},
		}

		if err := s.smtp.Send(email); err != nil {
			failedCount++
			if logErr := s.db.LogSend(campaignID, sub.ID, "failed", err.Error()); logErr != nil {
				log.Printf("failed to log send: %v", logErr)
			}
			// Mark as bounced on permanent SMTP failures (5xx codes)
			errStr := err.Error()
			if strings.Contains(errStr, "550") || strings.Contains(errStr, "551") ||
				strings.Contains(errStr, "552") || strings.Contains(errStr, "553") ||
				strings.Contains(errStr, "554") || strings.Contains(errStr, "User unknown") ||
				strings.Contains(errStr, "mailbox not found") || strings.Contains(errStr, "does not exist") {
				if bounceErr := s.db.MarkBounced(sub.ID); bounceErr != nil {
					log.Printf("failed to mark bounce for %s: %v", sub.Email, bounceErr)
				}
			}
			if s.onSend != nil {
				s.onSend(sub.Email, i+1, total, err)
			}
		} else {
			sentCount++
			if logErr := s.db.LogSend(campaignID, sub.ID, "sent", ""); logErr != nil {
				log.Printf("failed to log send: %v", logErr)
			}
			if s.onSend != nil {
				s.onSend(sub.Email, i+1, total, nil)
			}
		}

		// Rate limit: small delay between sends to avoid SMTP throttling
		time.Sleep(100 * time.Millisecond)
	}

	status := "sent"
	if sentCount == 0 {
		status = "failed"
	} else if failedCount > 0 {
		status = "partial"
	}

	if err := s.db.UpdateCampaignStatus(campaignID, status); err != nil {
		log.Printf("failed to update campaign status: %v", err)
	}
	if err := s.db.UpdateCampaignStats(campaignID, sentCount, failedCount); err != nil {
		log.Printf("failed to update campaign stats: %v", err)
	}

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
	return fmt.Sprintf("%s://%s/api/v1/unsubscribe?token=%s", scheme, domain, url.QueryEscape(token))
}
