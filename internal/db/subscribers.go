package db

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/google/uuid"
)

// AddSubscriber creates a subscriber with the given status.
// Use "pending" for double opt-in (requires confirmation) or "active" for direct add (CLI/import).
func (db *DB) AddSubscriber(email, name, listID string) (*Subscriber, error) {
	return db.AddSubscriberWithStatus(email, name, listID, "active")
}

func (db *DB) AddSubscriberWithStatus(email, name, listID, status string) (*Subscriber, error) {
	token := generateToken()
	confirmed := status == "active"
	sub := &Subscriber{
		ID:           uuid.New().String(),
		Email:        email,
		Name:         name,
		ListID:       listID,
		Status:       status,
		ConfirmToken: token,
		Confirmed:    confirmed,
	}

	_, err := db.conn.Exec(
		`INSERT INTO subscribers (id, email, name, list_id, status, confirm_token, confirmed)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.Email, sub.Name, sub.ListID, sub.Status, sub.ConfirmToken, confirmed,
	)
	if err != nil {
		return nil, fmt.Errorf("adding subscriber: %w", err)
	}
	return sub, nil
}

func (db *DB) GetSubscriber(id string) (*Subscriber, error) {
	sub := &Subscriber{}
	err := db.conn.QueryRow(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE id = ?`, id,
	).Scan(&sub.ID, &sub.Email, &sub.Name, &sub.ListID, &sub.Status, &sub.ConfirmToken,
		&sub.Confirmed, &sub.Metadata, &sub.SubscribedAt, &sub.UnsubscribedAt, &sub.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting subscriber: %w", err)
	}
	return sub, nil
}

func (db *DB) GetSubscriberByEmail(email, listID string) (*Subscriber, error) {
	sub := &Subscriber{}
	err := db.conn.QueryRow(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE email = ? AND list_id = ?`, email, listID,
	).Scan(&sub.ID, &sub.Email, &sub.Name, &sub.ListID, &sub.Status, &sub.ConfirmToken,
		&sub.Confirmed, &sub.Metadata, &sub.SubscribedAt, &sub.UnsubscribedAt, &sub.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting subscriber: %w", err)
	}
	return sub, nil
}

func (db *DB) ListSubscribers(listID string) ([]Subscriber, error) {
	rows, err := db.conn.Query(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE list_id = ? ORDER BY created_at DESC`, listID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing subscribers: %w", err)
	}
	defer rows.Close()

	var subs []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ID, &s.Email, &s.Name, &s.ListID, &s.Status, &s.ConfirmToken,
			&s.Confirmed, &s.Metadata, &s.SubscribedAt, &s.UnsubscribedAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning subscriber: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subscribers: %w", err)
	}
	return subs, nil
}

func (db *DB) ListSubscribersPaginated(listID string, limit, offset int) ([]Subscriber, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := db.conn.Query(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE list_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?`, listID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("listing subscribers: %w", err)
	}
	defer rows.Close()

	var subs []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ID, &s.Email, &s.Name, &s.ListID, &s.Status, &s.ConfirmToken,
			&s.Confirmed, &s.Metadata, &s.SubscribedAt, &s.UnsubscribedAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning subscriber: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subscribers: %w", err)
	}
	return subs, nil
}

func (db *DB) GetActiveSubscribers(listID string) ([]Subscriber, error) {
	rows, err := db.conn.Query(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE list_id = ? AND status = 'active' ORDER BY created_at DESC`, listID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing active subscribers: %w", err)
	}
	defer rows.Close()

	var subs []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ID, &s.Email, &s.Name, &s.ListID, &s.Status, &s.ConfirmToken,
			&s.Confirmed, &s.Metadata, &s.SubscribedAt, &s.UnsubscribedAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning subscriber: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subscribers: %w", err)
	}
	return subs, nil
}

func (db *DB) UnsubscribeByToken(token string) error {
	result, err := db.conn.Exec(
		`UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = CURRENT_TIMESTAMP
		 WHERE confirm_token = ? AND status = 'active'`, token,
	)
	if err != nil {
		return fmt.Errorf("unsubscribing: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("subscriber not found or already unsubscribed")
	}
	return nil
}

func (db *DB) UnsubscribeByEmail(email, listID string) error {
	_, err := db.conn.Exec(
		`UPDATE subscribers SET status = 'unsubscribed', unsubscribed_at = CURRENT_TIMESTAMP
		 WHERE email = ? AND list_id = ? AND status = 'active'`, email, listID,
	)
	return err
}

func (db *DB) DeleteSubscriber(id string) error {
	_, err := db.conn.Exec(`DELETE FROM subscribers WHERE id = ?`, id)
	return err
}

func (db *DB) ConfirmSubscriber(token string) error {
	result, err := db.conn.Exec(
		`UPDATE subscribers SET confirmed = 1, status = 'active' WHERE confirm_token = ? AND status IN ('pending', 'active')`, token,
	)
	if err != nil {
		return fmt.Errorf("confirming subscriber: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("invalid confirmation token")
	}
	return nil
}

// MarkBounced marks a subscriber as bounced so they won't receive future emails.
func (db *DB) MarkBounced(subscriberID string) error {
	_, err := db.conn.Exec(
		`UPDATE subscribers SET status = 'bounced' WHERE id = ? AND status = 'active'`,
		subscriberID,
	)
	return err
}

// GetSubscriberByToken retrieves a subscriber by their confirmation token.
func (db *DB) GetSubscriberByToken(token string) (*Subscriber, error) {
	sub := &Subscriber{}
	err := db.conn.QueryRow(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE confirm_token = ?`, token,
	).Scan(&sub.ID, &sub.Email, &sub.Name, &sub.ListID, &sub.Status, &sub.ConfirmToken,
		&sub.Confirmed, &sub.Metadata, &sub.SubscribedAt, &sub.UnsubscribedAt, &sub.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting subscriber by token: %w", err)
	}
	return sub, nil
}

// SearchSubscribers finds subscribers by email or name prefix.
func (db *DB) SearchSubscribers(listID, query string, limit int) ([]Subscriber, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	pattern := query + "%"
	rows, err := db.conn.Query(
		`SELECT id, email, name, list_id, status, confirm_token, confirmed, metadata, subscribed_at, unsubscribed_at, created_at
		 FROM subscribers WHERE list_id = ? AND (email LIKE ? OR name LIKE ?) ORDER BY email LIMIT ?`,
		listID, pattern, pattern, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("searching subscribers: %w", err)
	}
	defer rows.Close()

	var subs []Subscriber
	for rows.Next() {
		var s Subscriber
		if err := rows.Scan(&s.ID, &s.Email, &s.Name, &s.ListID, &s.Status, &s.ConfirmToken,
			&s.Confirmed, &s.Metadata, &s.SubscribedAt, &s.UnsubscribedAt, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning subscriber: %w", err)
		}
		subs = append(subs, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating subscribers: %w", err)
	}
	return subs, nil
}

func (db *DB) ImportSubscribers(listID string, entries []struct{ Email, Name string }) (int, error) {
	tx, err := db.conn.Begin()
	if err != nil {
		return 0, fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(
		`INSERT OR IGNORE INTO subscribers (id, email, name, list_id, status, confirm_token)
		 VALUES (?, ?, ?, ?, 'active', ?)`,
	)
	if err != nil {
		return 0, fmt.Errorf("preparing statement: %w", err)
	}
	defer stmt.Close()

	count := 0
	for _, e := range entries {
		result, err := stmt.Exec(uuid.New().String(), e.Email, e.Name, listID, generateToken())
		if err != nil {
			continue
		}
		rows, _ := result.RowsAffected()
		count += int(rows)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("committing transaction: %w", err)
	}
	return count, nil
}

// ResubscribePending sets an unsubscribed/bounced subscriber back to pending for double opt-in.
func (db *DB) ResubscribePending(id string) error {
	result, err := db.conn.Exec(
		`UPDATE subscribers SET status = 'pending', confirmed = 0, unsubscribed_at = NULL, confirm_token = ?
		 WHERE id = ? AND status IN ('unsubscribed', 'bounced')`,
		generateToken(), id,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("subscriber not in unsubscribed or bounced state")
	}
	return nil
}

// ResubscribeActive sets an unsubscribed/bounced subscriber back to active directly.
func (db *DB) ResubscribeActive(id string) error {
	result, err := db.conn.Exec(
		`UPDATE subscribers SET status = 'active', confirmed = 1, unsubscribed_at = NULL
		 WHERE id = ? AND status IN ('unsubscribed', 'bounced')`,
		id,
	)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("subscriber not in unsubscribed or bounced state")
	}
	return nil
}

func generateToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure indicates a critical system problem (e.g. /dev/urandom unavailable).
		// Generating predictable tokens would be a security vulnerability, so we panic.
		panic(fmt.Sprintf("inkdrift: crypto/rand.Read failed: %v", err))
	}
	return hex.EncodeToString(b)
}
