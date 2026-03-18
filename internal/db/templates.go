package db

import (
	"fmt"

	"github.com/google/uuid"
)

const maxTemplateBodySize = 2 << 20 // 2MB

func (db *DB) CreateTemplate(name, body string) (*Template, error) {
	if len(body) > maxTemplateBodySize {
		return nil, fmt.Errorf("template body too large (max %d bytes)", maxTemplateBodySize)
	}

	t := &Template{
		ID:   uuid.New().String(),
		Name: name,
		Body: body,
	}

	_, err := db.conn.Exec(
		`INSERT INTO templates (id, name, body) VALUES (?, ?, ?)`,
		t.ID, t.Name, t.Body,
	)
	if err != nil {
		return nil, fmt.Errorf("creating template: %w", err)
	}
	return t, nil
}

func (db *DB) GetTemplate(id string) (*Template, error) {
	t := &Template{}
	err := db.conn.QueryRow(
		`SELECT id, name, body, created_at, updated_at FROM templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting template: %w", err)
	}
	return t, nil
}

func (db *DB) GetTemplateByName(name string) (*Template, error) {
	t := &Template{}
	err := db.conn.QueryRow(
		`SELECT id, name, body, created_at, updated_at FROM templates WHERE name = ?`, name,
	).Scan(&t.ID, &t.Name, &t.Body, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting template: %w", err)
	}
	return t, nil
}

func (db *DB) ListTemplates() ([]Template, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, body, created_at, updated_at FROM templates ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Name, &t.Body, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning template: %w", err)
		}
		templates = append(templates, t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating templates: %w", err)
	}
	return templates, nil
}

func (db *DB) UpdateTemplate(id, name, body string) error {
	_, err := db.conn.Exec(
		`UPDATE templates SET name = ?, body = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		name, body, id,
	)
	return err
}

func (db *DB) DeleteTemplate(id string) error {
	_, err := db.conn.Exec(`DELETE FROM templates WHERE id = ?`, id)
	return err
}
