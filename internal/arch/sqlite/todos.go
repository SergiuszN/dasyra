package sqlite

import (
	"context"
	"time"

	"dasyra/internal/dom"
)

type Todos struct{ DB *DB }

const todoColumns = "id, text, day, done_at, created_at"

func (r Todos) list(ctx context.Context, query string, args ...any) ([]dom.Todo, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var todos []dom.Todo
	for rows.Next() {
		var t dom.Todo
		var done, created wall
		if err := rows.Scan(&t.ID, &t.Text, &t.Day, &done, &created); err != nil {
			return nil, err
		}
		t.DoneAt, t.CreatedAt = done.ptr(r.DB.Server), created.in(r.DB.Server)
		todos = append(todos, t)
	}
	return todos, rows.Err()
}

func (r Todos) Insert(ctx context.Context, userID int64, t *dom.Todo) error {
	_, err := r.DB.ExecContext(ctx, "INSERT INTO todos (id, user_id, text, day, done_at, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.ID, userID, t.Text, t.Day, nullableTime(t.DoneAt, r.DB.Server), r.DB.stamp(t.CreatedAt))
	return err
}

func (r Todos) Update(ctx context.Context, userID int64, t *dom.Todo) error {
	_, err := r.DB.ExecContext(ctx, "UPDATE todos SET text = ?, day = ?, done_at = ? WHERE id = ? AND user_id = ?",
		t.Text, t.Day, nullableTime(t.DoneAt, r.DB.Server), t.ID, userID)
	return err
}

func (r Todos) Delete(ctx context.Context, userID, id int64) error {
	_, err := r.DB.ExecContext(ctx, "DELETE FROM todos WHERE id = ? AND user_id = ?", id, userID)
	return err
}

func (r Todos) Get(ctx context.Context, userID, id int64) (*dom.Todo, error) {
	found, err := r.list(ctx, "SELECT "+todoColumns+" FROM todos WHERE id = ? AND user_id = ?", id, userID)
	if err != nil || len(found) == 0 {
		return nil, err
	}
	return &found[0], nil
}

func (r Todos) Current(ctx context.Context, userID int64, doneSince time.Time, limit int) ([]dom.Todo, error) {
	return r.list(ctx, "SELECT "+todoColumns+` FROM todos WHERE user_id = ? AND (done_at IS NULL OR done_at >= ?)
		ORDER BY done_at IS NULL, day = '', day, id LIMIT ?`, userID, r.DB.stamp(doneSince), limit)
}
