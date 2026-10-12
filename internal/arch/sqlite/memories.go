package sqlite

import (
	"context"
	"database/sql"
	"time"

	"dasyra/internal/dom"
)

// Memories archives every version it overwrites or deletes to memories_archive, so upkeep never loses data.
type Memories struct{ DB *DB }

const memoryColumns = "m.id, m.name, m.category, m.content, m.created_at, m.updated_at"

func (r Memories) list(ctx context.Context, query string, args ...any) ([]dom.Memory, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var memories []dom.Memory
	for rows.Next() {
		var m dom.Memory
		var created, updated wall
		if err := rows.Scan(&m.ID, &m.Name, &m.Category, &m.Content, &created, &updated); err != nil {
			return nil, err
		}
		m.CreatedAt, m.UpdatedAt = created.in(r.DB.Server), updated.in(r.DB.Server)
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (r Memories) Insert(ctx context.Context, userID int64, m *dom.Memory) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO memories (id, user_id, name, category, content, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		m.ID, userID, m.Name, m.Category, m.Content, r.DB.stamp(m.CreatedAt), r.DB.stamp(m.UpdatedAt))
	return err
}

func (r Memories) Update(ctx context.Context, userID int64, m *dom.Memory) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if err := r.archive(ctx, tx, userID, m.ID, "update"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE memories SET name = ?, category = ?, content = ?, updated_at = ? WHERE id = ? AND user_id = ?",
			m.Name, m.Category, m.Content, r.DB.stamp(m.UpdatedAt), m.ID, userID)
		return err
	})
}

func (r Memories) Delete(ctx context.Context, userID, id int64) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if err := r.archive(ctx, tx, userID, id, "delete"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM memories WHERE id = ? AND user_id = ?", id, userID)
		return err
	})
}

func (r Memories) archive(ctx context.Context, tx *sql.Tx, userID, id int64, reason string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO memories_archive (id, user_id, name, category, content, created_at, updated_at, archived_at, reason)
		SELECT id, user_id, name, category, content, created_at, updated_at, ?, ? FROM memories WHERE id = ? AND user_id = ?`,
		r.DB.stamp(time.Now()), reason, id, userID)
	return err
}

func (r Memories) Get(ctx context.Context, userID, id int64) (*dom.Memory, error) {
	found, err := r.list(ctx, "SELECT "+memoryColumns+" FROM memories m WHERE m.id = ? AND m.user_id = ?", id, userID)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

func (r Memories) Search(ctx context.Context, userID int64, terms []string, category string, offset, limit int) ([]dom.Memory, error) {
	ftsQuery, err := r.DB.match(ctx, memoriesFTS, userID, terms)
	if err != nil || ftsQuery == "" {
		return nil, err
	}
	return r.list(ctx, "SELECT "+memoryColumns+` FROM memories_fts JOIN memories m ON memories_fts.rowid = m.id
		WHERE memories_fts MATCH ? AND m.user_id = ? AND (? = '' OR m.category = ?)
		ORDER BY bm25(memories_fts, 2.0, 1.0, 0.5) ASC, m.updated_at DESC LIMIT ? OFFSET ?`,
		ftsQuery, userID, category, category, limit, offset)
}

func (r Memories) List(ctx context.Context, userID int64, category string, offset, limit int) ([]dom.Memory, error) {
	return r.list(ctx, "SELECT "+memoryColumns+` FROM memories m WHERE m.user_id = ? AND (? = '' OR m.category = ?)
		ORDER BY m.updated_at DESC LIMIT ? OFFSET ?`, userID, category, category, limit, offset)
}
