package db

import (
	"fmt"

	"github.com/google/uuid"
)

func (db *DB) CreateList(name, description string) (*List, error) {
	list := &List{
		ID:          uuid.New().String(),
		Name:        name,
		Description: description,
	}

	_, err := db.conn.Exec(
		`INSERT INTO lists (id, name, description) VALUES (?, ?, ?)`,
		list.ID, list.Name, list.Description,
	)
	if err != nil {
		return nil, fmt.Errorf("creating list: %w", err)
	}
	return list, nil
}

func (db *DB) GetList(id string) (*List, error) {
	list := &List{}
	err := db.conn.QueryRow(
		`SELECT id, name, description, created_at, updated_at FROM lists WHERE id = ?`, id,
	).Scan(&list.ID, &list.Name, &list.Description, &list.CreatedAt, &list.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting list: %w", err)
	}
	return list, nil
}

func (db *DB) GetListByName(name string) (*List, error) {
	list := &List{}
	err := db.conn.QueryRow(
		`SELECT id, name, description, created_at, updated_at FROM lists WHERE name = ?`, name,
	).Scan(&list.ID, &list.Name, &list.Description, &list.CreatedAt, &list.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("getting list: %w", err)
	}
	return list, nil
}

func (db *DB) ListLists() ([]List, error) {
	rows, err := db.conn.Query(
		`SELECT id, name, description, created_at, updated_at FROM lists ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing lists: %w", err)
	}
	defer rows.Close()

	var lists []List
	for rows.Next() {
		var l List
		if err := rows.Scan(&l.ID, &l.Name, &l.Description, &l.CreatedAt, &l.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning list: %w", err)
		}
		lists = append(lists, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating lists: %w", err)
	}
	return lists, nil
}

func (db *DB) DeleteList(id string) error {
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM subscribers WHERE list_id = ?`, id); err != nil {
		return fmt.Errorf("deleting list subscribers: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM lists WHERE id = ?`, id); err != nil {
		return fmt.Errorf("deleting list: %w", err)
	}
	return tx.Commit()
}

func (db *DB) ListSubscriberCount(listID string) (int, error) {
	var count int
	err := db.conn.QueryRow(
		`SELECT COUNT(*) FROM subscribers WHERE list_id = ? AND status = 'active'`, listID,
	).Scan(&count)
	return count, err
}
