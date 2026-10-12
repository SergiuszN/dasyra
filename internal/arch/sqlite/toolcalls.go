package sqlite

import (
	"context"
	"database/sql"
	"time"

	"dasyra/internal/dom"
)

const toolCallsKeptFor = 48 * time.Hour

type ToolCalls struct{ DB *DB }

// SaveRun stores the calls behind a reply and drops the user's calls older than two days.
func (r ToolCalls) SaveRun(ctx context.Context, userID, replyID int64, calls []dom.ToolCall) error {
	now := time.Now()
	return r.DB.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, "DELETE FROM tool_calls WHERE user_id = ? AND created_at < ?", userID, r.DB.stamp(now.Add(-toolCallsKeptFor))); err != nil {
			return err
		}
		for _, c := range calls {
			if _, err := tx.ExecContext(ctx, "INSERT INTO tool_calls (user_id, conversation_id, tool, args, result, created_at) VALUES (?, ?, ?, ?, ?, ?)",
				userID, replyID, c.Tool, c.Args, c.Result, r.DB.stamp(now)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r ToolCalls) LatestRun(ctx context.Context, userID int64) ([]dom.ToolCall, error) {
	rows, err := r.DB.QueryContext(ctx, `SELECT conversation_id, tool, args, result, created_at FROM tool_calls
		WHERE user_id = ?1 AND conversation_id = (SELECT MAX(conversation_id) FROM tool_calls WHERE user_id = ?1) ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var calls []dom.ToolCall
	for rows.Next() {
		var c dom.ToolCall
		var created wall
		if err := rows.Scan(&c.ReplyID, &c.Tool, &c.Args, &c.Result, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = created.in(r.DB.Server)
		calls = append(calls, c)
	}
	return calls, rows.Err()
}
