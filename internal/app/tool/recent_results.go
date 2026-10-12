package tool

import (
	"context"
	"encoding/json"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

const (
	RecentResultsName   = "recent_tool_results"
	maxRecentResultRune = 6000
)

// RecentResults gives the full results behind the latest reply that used tools, so a follow-up doesn't repeat them.
type RecentResults struct {
	ToolCalls dom.ToolCalls
}

func (RecentResults) Spec() llm.Tool {
	return Spec(RecentResultsName, "Full results of the tools you used for your latest reply that used any (web searches, fetched pages, memory and calendar lookups). Check it before repeating the same search for a follow-up question.")
}

type recentCall struct {
	Tool   string          `json:"tool"`
	Args   json.RawMessage `json:"args"`
	Result string          `json:"result"`
}

func (t RecentResults) Run(ctx context.Context, s *Session, _ Args) (string, error) {
	calls, err := t.ToolCalls.LatestRun(ctx, s.User.ID)
	if err != nil {
		return "", err
	}
	if len(calls) == 0 {
		return "No recent tool results.", nil
	}

	out := struct {
		ReplyAt string       `json:"reply_at"`
		Calls   []recentCall `json:"calls"`
	}{ReplyAt: calls[0].CreatedAt.Format(dom.DateTimeLayout)}
	for _, c := range calls {
		out.Calls = append(out.Calls, recentCall{c.Tool, json.RawMessage(c.Args), cut(c.Result, maxRecentResultRune, "… [cut]")})
	}
	return toJSON(out), nil
}

// cut shortens text to max runes, marking the cut.
func cut(text string, max int, mark string) string {
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max]) + mark
}
