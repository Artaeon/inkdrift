package db

import (
	"fmt"

	"github.com/google/uuid"
)

const (
	maxCampaignBodySize = 1 << 20  // 1MB
	maxSubjectSize      = 998      // RFC 5322 max header line length
)

func (db *DB) CreateCampaign(name, subject, body, listID string) (*Campaign, error) {
	if len(body) > maxCampaignBodySize {
		return nil, fmt.Errorf("campaign body too large (max %d bytes)", maxCampaignBodySize)
	}
	if len(subject) > maxSubjectSize {
		return nil, fmt.Errorf("subject too long (max %d chars)", maxSubjectSize)
	}

	c := &Campaign{
		ID:      uuid.New().String(),
		Name:    name,
		Subject: subject,
		Body:    body,
		ListID:  listID,
		Status:  "draft",
	}

	_, err := db.conn.Exec(
		`INSERT INTO campaigns (id, name, subject, body, list_id, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Subject, c.Body, c.ListID, c.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("creating campaign: %w", err)
	}
	return c, nil
}

func (db *DB) GetCampaign(id string) (*Campaign, error) {
	c := &Campaign{}
	err := db.conn.QueryRow(
		`SELECT id, name, subject, body, list_id, status, template_id, sent_at,
		        sent_count, failed_count, open_count, click_count, created_at, updated_at
		 FROM campaigns WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Subject, &c.Body, &c.ListID, &c.Status, &c.TemplateID,
		&c.SentAt, &c.SentCount, &c.FailedCount, &c.OpenCount, &c.ClickCount,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting campaign: %w", err)
	}
	return c, nil
}

func (db *DB) ListCampaigns() ([]Campaign, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, subject, body, list_id, status, template_id, sent_at,
		        sent_count, failed_count, open_count, click_count, created_at, updated_at
		 FROM campaigns ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing campaigns: %w", err)
	}
	defer rows.Close()

	var campaigns []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.Name, &c.Subject, &c.Body, &c.ListID, &c.Status, &c.TemplateID,
			&c.SentAt, &c.SentCount, &c.FailedCount, &c.OpenCount, &c.ClickCount,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning campaign: %w", err)
		}
		campaigns = append(campaigns, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating campaigns: %w", err)
	}
	return campaigns, nil
}

// validStatuses guards against invalid status values
var validStatuses = map[string]bool{"draft": true, "sending": true, "sent": true, "failed": true}

func (db *DB) UpdateCampaignStatus(id, status string) error {
	if !validStatuses[status] {
		return fmt.Errorf("invalid campaign status: %s", status)
	}
	_, err := db.conn.Exec(
		`UPDATE campaigns SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	return err
}

// ClaimCampaignForSending atomically transitions a campaign from draft to sending.
// Returns an error if the campaign is not in draft status (prevents double-send race).
func (db *DB) ClaimCampaignForSending(id string) error {
	result, err := db.conn.Exec(
		`UPDATE campaigns SET status = 'sending', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'draft'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("claiming campaign: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("campaign is not in draft status or does not exist")
	}
	return nil
}

func (db *DB) UpdateCampaignStats(id string, sentCount, failedCount int) error {
	_, err := db.conn.Exec(
		`UPDATE campaigns SET sent_count = ?, failed_count = ?, sent_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sentCount, failedCount, id,
	)
	return err
}

func (db *DB) UpdateCampaignBody(id, subject, body string) (*Campaign, error) {
	_, err := db.conn.Exec(
		`UPDATE campaigns SET subject = ?, body = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND status = 'draft'`,
		subject, body, id,
	)
	if err != nil {
		return nil, fmt.Errorf("updating campaign: %w", err)
	}
	return db.GetCampaign(id)
}

func (db *DB) SetCampaignTemplate(id, templateID string) error {
	_, err := db.conn.Exec(
		`UPDATE campaigns SET template_id = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		templateID, id,
	)
	return err
}

func (db *DB) DeleteCampaign(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM send_log WHERE campaign_id = ?`, id); err != nil {
		return fmt.Errorf("deleting send logs: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM campaigns WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting campaign: %w", err)
	}
	return tx.Commit()
}

func (db *DB) LogSend(campaignID, subscriberID, status, errMsg string) error {
	_, err := db.conn.Exec(
		`INSERT INTO send_log (id, campaign_id, subscriber_id, status, error)
		 VALUES (?, ?, ?, ?, ?)`,
		uuid.New().String(), campaignID, subscriberID, status, errMsg,
	)
	return err
}
