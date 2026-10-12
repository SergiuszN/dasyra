package tool

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"dasyra/internal/app/llm"
)

const (
	pageLimit     = 60000
	documentLimit = 150000 // a PDF is usually the whole point of the fetch (a price list, a menu, a report)
	defaultAsk    = "Summarize the main content: key facts, numbers, dates and names."
)

// TextReader answers a question about a long text in a separate cheap pass, so raw content never reaches Dasyra.
type TextReader struct {
	AI  llm.Client
	Log *slog.Logger
}

// Answer reads text of the given kind ("page" or "document"), a missing question means a summary.
func (r TextReader) Answer(ctx context.Context, text, question string, limit int, kind string) string {
	if question == "" {
		question = defaultAsk
	}
	runes := []rune(text)
	truncated := len(runes) > limit
	if truncated {
		text = string(runes[:limit])
	}

	reply, err := r.AI.Chat(llm.For(ctx, 0, llm.PurposeTextReader), llm.Request{Model: llm.TextReader, Messages: []llm.Message{
		llm.System(fmt.Sprintf("Use only the provided %[1]s to answer the question. Answer fully focusing on the question, but if the answer is not in the %[1]s, say this. Don't use any outside knowledge.", kind)),
		llm.User(capitalize(kind) + " content: " + text),
		llm.User("Question: " + question),
	}})
	if err != nil {
		r.Log.Error("Invalid response from TextReader AI", "error", err)
		return fmt.Sprintf("I had trouble reading the %s. Please try again.", kind)
	}
	if truncated {
		return fmt.Sprintf("%s\n\n(Only the first %d characters of the %s were read.)", reply.Text, limit, kind)
	}
	return reply.Text
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + strings.TrimPrefix(s, string(r[0]))
}
