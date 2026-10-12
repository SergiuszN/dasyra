package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"dasyra/internal/dom"
)

type Messages struct{ DB *DB }

const messageColumns = "c.id, c.role, c.content, COALESCE(c.telegram_message_id, 0), c.created_at, COALESCE(c.image_file_id, ''), COALESCE(c.document_file_id, ''), c.check_in"

func (r Messages) list(ctx context.Context, query string, args ...any) ([]dom.Message, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var messages []dom.Message
	for rows.Next() {
		var m dom.Message
		var created wall
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.TelegramID, &created, &m.ImageFileID, &m.DocumentFileID, &m.CheckIn); err != nil {
			return nil, err
		}
		m.CreatedAt = created.in(r.DB.Server)
		messages = append(messages, m)
	}
	return messages, rows.Err()
}

func (r Messages) Insert(ctx context.Context, userID int64, m *dom.Message) error {
	var telegramID any
	if m.TelegramID != 0 {
		telegramID = m.TelegramID
	}
	_, err := r.DB.ExecContext(ctx,
		"INSERT INTO conversations (id, user_id, role, content, telegram_message_id, created_at, image_file_id, document_file_id, check_in) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		m.ID, userID, m.Role, m.Content, telegramID, r.DB.stamp(m.CreatedAt), nullable(m.ImageFileID), nullable(m.DocumentFileID), m.CheckIn)
	return err
}

func (r Messages) Last(ctx context.Context, userID int64, n int) (dom.History, error) {
	return r.list(ctx, "SELECT "+messageColumns+" FROM conversations c WHERE c.user_id = ? ORDER BY c.created_at DESC, c.id DESC LIMIT ?", userID, n)
}

func (r Messages) Count(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM conversations WHERE user_id = ?", userID).Scan(&n)
	return n, err
}

func (r Messages) After(ctx context.Context, userID int64, after time.Time, limit int) ([]dom.Message, error) {
	return r.list(ctx, "SELECT * FROM (SELECT "+messageColumns+" FROM conversations c WHERE c.user_id = ? AND c.created_at > ? ORDER BY c.created_at DESC LIMIT ?) ORDER BY created_at ASC",
		userID, r.DB.stamp(after), limit)
}

func (r Messages) Search(ctx context.Context, userID int64, terms []string, limit int, from, to *time.Time) ([]dom.Message, error) {
	ftsQuery, err := r.DB.match(ctx, conversationsFTS, userID, terms)
	if err != nil || ftsQuery == "" {
		return nil, err
	}
	query := "SELECT " + messageColumns + " FROM conversations_fts JOIN conversations c ON conversations_fts.rowid = c.id WHERE conversations_fts MATCH ? AND c.user_id = ?"
	args := []any{ftsQuery, userID}
	if from != nil {
		query, args = query+" AND c.created_at >= ?", append(args, r.DB.stamp(*from))
	}
	if to != nil {
		query, args = query+" AND c.created_at <= ?", append(args, r.DB.stamp(*to))
	}
	return r.list(ctx, query+" ORDER BY bm25(conversations_fts, 0.5, 1.5) ASC, c.created_at DESC LIMIT ?", append(args, limit)...)
}

func (r Messages) Some(ctx context.Context, userID int64, ids []int64) ([]dom.Message, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	args := []any{userID}
	for _, id := range ids {
		args = append(args, id)
	}
	return r.list(ctx, "SELECT "+messageColumns+" FROM conversations c WHERE c.user_id = ? AND c.id IN (?"+strings.Repeat(", ?", len(ids)-1)+")", args...)
}

func (r Messages) HasUserMessageAfter(ctx context.Context, userID, messageID int64) (bool, error) {
	return r.exists(ctx, "SELECT COUNT(*) FROM conversations WHERE user_id = ? AND role = 'user' AND id > ?", userID, messageID)
}

func (r Messages) CheckInsSinceUserMessage(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM conversations WHERE user_id = ?1 AND check_in = 1
		AND created_at > COALESCE((SELECT MAX(created_at) FROM conversations WHERE user_id = ?1 AND role = 'user'), '')`, userID).Scan(&n)
	return n, err
}

func (r Messages) LastUserMessageAt(ctx context.Context, userID int64) (time.Time, error) {
	var last wall
	err := r.DB.QueryRowContext(ctx, "SELECT MAX(created_at) FROM conversations WHERE user_id = ? AND role = 'user'", userID).Scan(&last)
	if err != nil || !last.valid {
		return time.Time{}, err
	}
	return last.in(r.DB.Server), nil
}

func (r Messages) DocumentFileID(ctx context.Context, userID, telegramID int64) (string, error) {
	var id string
	err := r.DB.QueryRowContext(ctx, `SELECT document_file_id FROM conversations
		WHERE user_id = ? AND telegram_message_id = ? AND document_file_id IS NOT NULL ORDER BY created_at DESC LIMIT 1`, userID, telegramID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// Seen is bounded in time: message ids restart in a new bot chat, an unbounded check would drop new messages
// that reuse ids from the old bot.
func (r Messages) Seen(ctx context.Context, userID, telegramID int64, since time.Time) (bool, error) {
	return r.exists(ctx, "SELECT COUNT(*) FROM conversations WHERE user_id = ? AND telegram_message_id = ? AND created_at > ?", userID, telegramID, r.DB.stamp(since))
}

func (r Messages) exists(ctx context.Context, query string, args ...any) (bool, error) {
	var n int
	err := r.DB.QueryRowContext(ctx, query, args...).Scan(&n)
	return n > 0, err
}
