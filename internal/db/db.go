package db

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	conn *sql.DB
}

type Subscriber struct {
	ID            string
	Email         string
	Name          string
	ListID        string
	Status        string // active, unsubscribed, bounced
	ConfirmToken  string
	Confirmed     bool
	Metadata      string // JSON
	SubscribedAt  time.Time
	UnsubscribedAt *time.Time
	CreatedAt     time.Time
}

type List struct {
	ID          string
	Name        string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Campaign struct {
	ID          string
	Name        string
	Subject     string
	Body        string
	ListID      string
	Status      string // draft, sending, sent, failed
	TemplateID  string
	SentAt      *time.Time
	SentCount   int
	FailedCount int
	OpenCount   int
	ClickCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Template struct {
	ID        string
	Name      string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrating database: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS lists (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			description TEXT DEFAULT '',
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS subscribers (
			id TEXT PRIMARY KEY,
			email TEXT NOT NULL,
			name TEXT DEFAULT '',
			list_id TEXT NOT NULL REFERENCES lists(id),
			status TEXT DEFAULT 'active',
			confirm_token TEXT DEFAULT '',
			confirmed INTEGER DEFAULT 0,
			metadata TEXT DEFAULT '{}',
			subscribed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			unsubscribed_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(email, list_id)
		)`,
		`CREATE TABLE IF NOT EXISTS campaigns (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			subject TEXT NOT NULL,
			body TEXT NOT NULL,
			list_id TEXT NOT NULL REFERENCES lists(id),
			status TEXT DEFAULT 'draft',
			template_id TEXT DEFAULT '',
			sent_at DATETIME,
			sent_count INTEGER DEFAULT 0,
			failed_count INTEGER DEFAULT 0,
			open_count INTEGER DEFAULT 0,
			click_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS templates (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			body TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS send_log (
			id TEXT PRIMARY KEY,
			campaign_id TEXT NOT NULL REFERENCES campaigns(id),
			subscriber_id TEXT NOT NULL REFERENCES subscribers(id),
			status TEXT DEFAULT 'pending',
			error TEXT DEFAULT '',
			sent_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_subscribers_email ON subscribers(email)`,
		`CREATE INDEX IF NOT EXISTS idx_subscribers_list ON subscribers(list_id)`,
		`CREATE INDEX IF NOT EXISTS idx_subscribers_status ON subscribers(status)`,
		`CREATE INDEX IF NOT EXISTS idx_campaigns_list ON campaigns(list_id)`,
		`CREATE INDEX IF NOT EXISTS idx_campaigns_status ON campaigns(status)`,
		`CREATE INDEX IF NOT EXISTS idx_send_log_campaign ON send_log(campaign_id)`,
	}

	for _, m := range migrations {
		if _, err := db.conn.Exec(m); err != nil {
			return fmt.Errorf("migration failed: %w\nSQL: %s", err, m)
		}
	}

	return nil
}
