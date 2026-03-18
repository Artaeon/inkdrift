package db

import (
	"fmt"

	"github.com/google/uuid"
)

func (db *DB) CreateCampaign(name, subject, body, listID string) (*Campaign, error) {
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
	return campaigns, nil
}

func (db *DB) UpdateCampaignStatus(id, status string) error {
	_, err := db.conn.Exec(
		`UPDATE campaigns SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		status, id,
	)
	return err
}

func (db *DB) UpdateCampaignStats(id string, sentCount, failedCount int) error {
	_, err := db.conn.Exec(
		`UPDATE campaigns SET sent_count = ?, failed_count = ?, sent_at = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		sentCount, failedCount, id,
	)
	return err
}

func (db *DB) DeleteCampaign(id string) error {
	_, err := db.conn.Exec(`DELETE FROM send_log WHERE campaign_id = ?`, id)
	if err != nil {
		return fmt.Errorf("deleting send logs: %w", err)
	}
	_, err = db.conn.Exec(`DELETE FROM campaigns WHERE id = ?`, id)
	return err
}

func (db *DB) LogSend(campaignID, subscriberID, status, errMsg string) error {
	_, err := db.conn.Exec(
		`INSERT INTO send_log (id, campaign_id, subscriber_id, status, error)
		 VALUES (?, ?, ?, ?, ?)`,
		uuid.New().String(), campaignID, subscriberID, status, errMsg,
	)
	return err
}
