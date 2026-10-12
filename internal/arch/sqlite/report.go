package sqlite

import (
	"context"
	"strings"
	"time"
)

// Report reads the AI cost accounting for the admin panel.
type Report struct{ DB *DB }

// Totals sums a set of AI calls.
type Totals struct {
	Calls                                                                           int
	Cost                                                                            float64
	PromptTokens, CachedTokens, CacheWriteTokens, CompletionTokens, ReasoningTokens int
	AudioSeconds                                                                    float64
	LastAt                                                                          string // server wall clock, "" without calls
	Estimated                                                                       int    // calls priced from the panel's prices
	Unpriced                                                                        int    // paid-looking calls with no cost at all: a price is missing
}

// Group is the totals of one user, purpose, model or day.
type Group struct {
	Name   string // purpose, model, day or user name
	UserID int64  // ByUser only, 0 for calls without a user
	Totals
}

const totalsColumns = `COUNT(*), COALESCE(SUM(cost), 0), COALESCE(SUM(prompt_tokens), 0), COALESCE(SUM(cached_tokens), 0),
	COALESCE(SUM(cache_write_tokens), 0), COALESCE(SUM(completion_tokens), 0), COALESCE(SUM(reasoning_tokens), 0),
	COALESCE(SUM(audio_seconds), 0), COALESCE(CAST(MAX(created_at) AS TEXT), ''), COALESCE(SUM(cost_estimated), 0),
	COALESCE(SUM(cost = 0 AND cost_estimated = 0 AND (prompt_tokens > 0 OR audio_seconds > 0)), 0)`

func (t *Totals) fields() []any {
	return []any{&t.Calls, &t.Cost, &t.PromptTokens, &t.CachedTokens, &t.CacheWriteTokens, &t.CompletionTokens, &t.ReasoningTokens, &t.AudioSeconds, &t.LastAt, &t.Estimated, &t.Unpriced}
}

// Totals of calls since from (nil for all time); user nil means everyone, a pointer to 0 the calls without a user.
func (r Report) Totals(ctx context.Context, from *time.Time, user *int64) (Totals, error) {
	where, args := r.where(from, user)
	var t Totals
	err := r.DB.QueryRowContext(ctx, "SELECT "+totalsColumns+" FROM ai_usage WHERE "+where, args...).Scan(t.fields()...)
	return t, err
}

// ByUser is sorted by cost, the most expensive first.
func (r Report) ByUser(ctx context.Context, from *time.Time) ([]Group, error) {
	where, args := r.where(from, nil)
	return r.groups(ctx, `SELECT COALESCE(user_id, 0), COALESCE((SELECT name FROM users WHERE users.id = ai_usage.user_id), ''), `+totalsColumns+`
		FROM ai_usage WHERE `+where+` GROUP BY user_id ORDER BY 4 DESC`, true, args...)
}

// Grouped sums by "purpose" or "model", the most expensive first.
func (r Report) Grouped(ctx context.Context, column string, from *time.Time, user *int64) ([]Group, error) {
	if column != "model" {
		column = "purpose"
	}
	where, args := r.where(from, user)
	return r.groups(ctx, "SELECT "+column+", "+totalsColumns+" FROM ai_usage WHERE "+where+" GROUP BY "+column+" ORDER BY 3 DESC", false, args...)
}

// Daily sums per server day, keyed by "2006-01-02"; days without calls are missing.
func (r Report) Daily(ctx context.Context, from *time.Time, user *int64) (map[string]Totals, error) {
	where, args := r.where(from, user)
	groups, err := r.groups(ctx, "SELECT date(created_at) AS day, "+totalsColumns+" FROM ai_usage WHERE "+where+" GROUP BY day ORDER BY day", false, args...)
	days := make(map[string]Totals, len(groups))
	for _, g := range groups {
		days[g.Name] = g.Totals
	}
	return days, err
}

// RepliesByUser counts Dasyra's stored replies (check-ins and digests too), to show the cost of one reply.
func (r Report) RepliesByUser(ctx context.Context, from time.Time) (map[int64]int, error) {
	rows, err := r.DB.QueryContext(ctx, "SELECT user_id, COUNT(*) FROM conversations WHERE role = 'assistant' AND created_at >= ? GROUP BY user_id", r.DB.stamp(from))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	replies := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		replies[id] = n
	}
	return replies, rows.Err()
}

// FirstAt is when accounting started, zero without calls.
func (r Report) FirstAt(ctx context.Context) (time.Time, error) {
	var first wall
	err := r.DB.QueryRowContext(ctx, "SELECT MIN(created_at) FROM ai_usage").Scan(&first)
	if err != nil || !first.valid {
		return time.Time{}, err
	}
	return first.in(r.DB.Server), nil
}

func (r Report) groups(ctx context.Context, query string, withUser bool, args ...any) ([]Group, error) {
	rows, err := r.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var groups []Group
	for rows.Next() {
		var g Group
		dest := append([]any{&g.Name}, g.Totals.fields()...)
		if withUser {
			dest = append([]any{&g.UserID}, dest...)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

func (r Report) where(from *time.Time, user *int64) (string, []any) {
	conditions, args := []string{"1 = 1"}, []any{}
	if from != nil {
		conditions, args = append(conditions, "created_at >= ?"), append(args, r.DB.stamp(*from))
	}
	switch {
	case user == nil:
	case *user == 0:
		conditions = append(conditions, "user_id IS NULL")
	default:
		conditions, args = append(conditions, "user_id = ?"), append(args, *user)
	}
	return strings.Join(conditions, " AND "), args
}
