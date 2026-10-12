package sqlite

import (
	"context"
	"database/sql"
	"time"

	"dasyra/internal/dom"
)

// Notes archives every version it overwrites or deletes to notes_archive, like Memories.
type Notes struct{ DB *DB }

const noteColumns = "n.id, n.collection, n.title, n.tags, n.content, n.source, n.created_at, n.updated_at"

func (r Notes) list(ctx context.Context, query string, args ...any) ([]dom.Note, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []dom.Note
	for rows.Next() {
		var n dom.Note
		var created, updated wall
		if err := rows.Scan(&n.ID, &n.Collection, &n.Title, &n.Tags, &n.Content, &n.Source, &created, &updated); err != nil {
			return nil, err
		}
		n.CreatedAt, n.UpdatedAt = created.in(r.DB.Server), updated.in(r.DB.Server)
		notes = append(notes, n)
	}
	return notes, rows.Err()
}

func (r Notes) Insert(ctx context.Context, userID int64, n *dom.Note) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO notes (id, user_id, collection, title, tags, content, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		n.ID, userID, n.Collection, n.Title, n.Tags, n.Content, n.Source, r.DB.stamp(n.CreatedAt), r.DB.stamp(n.UpdatedAt))
	return err
}

func (r Notes) Update(ctx context.Context, userID int64, n *dom.Note) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if err := r.archive(ctx, tx, userID, n.ID, "update"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "UPDATE notes SET collection = ?, title = ?, tags = ?, content = ?, source = ?, updated_at = ? WHERE id = ? AND user_id = ?",
			n.Collection, n.Title, n.Tags, n.Content, n.Source, r.DB.stamp(n.UpdatedAt), n.ID, userID)
		return err
	})
}

func (r Notes) Delete(ctx context.Context, userID, id int64) error {
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if err := r.archive(ctx, tx, userID, id, "delete"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, "DELETE FROM notes WHERE id = ? AND user_id = ?", id, userID)
		return err
	})
}

func (r Notes) archive(ctx context.Context, tx *sql.Tx, userID, id int64, reason string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO notes_archive (id, user_id, collection, title, tags, content, source, created_at, updated_at, archived_at, reason)
		SELECT id, user_id, collection, title, tags, content, source, created_at, updated_at, ?, ? FROM notes WHERE id = ? AND user_id = ?`,
		r.DB.stamp(time.Now()), reason, id, userID)
	return err
}

func (r Notes) Get(ctx context.Context, userID, id int64) (*dom.Note, error) {
	found, err := r.list(ctx, "SELECT "+noteColumns+" FROM notes n WHERE n.id = ? AND n.user_id = ?", id, userID)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

func (r Notes) Find(ctx context.Context, userID int64, terms []string, collection string, offset, limit int) ([]dom.Note, error) {
	if len(terms) == 0 {
		return r.list(ctx, "SELECT "+noteColumns+` FROM notes n WHERE n.user_id = ? AND (? = '' OR n.collection = ?)
			ORDER BY n.updated_at DESC LIMIT ? OFFSET ?`, userID, collection, collection, limit, offset)
	}
	ftsQuery, err := r.DB.match(ctx, notesFTS, userID, terms)
	if err != nil || ftsQuery == "" {
		return nil, err
	}
	// Title and tags say what a note is, the content only mentions things
	return r.list(ctx, "SELECT "+noteColumns+` FROM notes_fts JOIN notes n ON notes_fts.rowid = n.id
		WHERE notes_fts MATCH ? AND n.user_id = ? AND (? = '' OR n.collection = ?)
		ORDER BY bm25(notes_fts, 3.0, 2.0, 0.5) ASC, n.updated_at DESC LIMIT ? OFFSET ?`,
		ftsQuery, userID, collection, collection, limit, offset)
}

func (r Notes) Collections(ctx context.Context, userID int64) ([]dom.Collection, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT collection, COUNT(*) FROM notes WHERE user_id = ? GROUP BY collection ORDER BY MIN(created_at)", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var collections []dom.Collection
	for rows.Next() {
		var c dom.Collection
		if err := rows.Scan(&c.Name, &c.Count); err != nil {
			return nil, err
		}
		collections = append(collections, c)
	}
	return collections, rows.Err()
}
