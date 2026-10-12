package sqlite

import (
	"context"
	"time"

	"dasyra/internal/dom"
)

// UsageLog is the cost and token accounting of AI calls, the only table not scoped by user: it serves the admin panel.
type UsageLog struct{ DB *DB }

func (r UsageLog) Record(ctx context.Context, u dom.Usage) error {
	var userID any
	if u.UserID != 0 {
		userID = u.UserID
	}
	_, err := r.DB.ExecContext(ctx, `INSERT INTO ai_usage (user_id, purpose, model, prompt_tokens, cached_tokens, cache_write_tokens, completion_tokens, reasoning_tokens, audio_seconds, cost, cost_estimated, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		userID, u.Purpose, u.Model, u.PromptTokens, u.CachedTokens, u.CacheWriteTokens, u.CompletionTokens, u.ReasoningTokens, u.AudioSeconds, u.Cost, u.Estimated, r.DB.stamp(time.Now()))
	return err
}
