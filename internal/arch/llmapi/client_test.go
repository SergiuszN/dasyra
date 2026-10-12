package llmapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type usageLog struct{ records []dom.Usage }

func (u *usageLog) Record(_ context.Context, r dom.Usage) error {
	u.records = append(u.records, r)
	return nil
}

func TestCacheBreakpointsOnlyForAnthropic(t *testing.T) {
	messages := []llm.Message{
		llm.System("persona").Cached(),
		llm.User("volatile"),
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "c1", Name: "list_memories", Arguments: "{}"}}},
		{Role: "tool", ToolCallID: "c1", Text: "result"},
	}
	wire := encodeMessages("anthropic/claude-haiku-5.5", messages)
	if !hasBreakpoint(wire[0]) || hasBreakpoint(wire[1]) || !hasBreakpoint(wire[3]) {
		t.Fatalf("breakpoints: %+v", wire)
	}
	if wire[2].Content != nil || wire[2].ToolCalls[0].Type != "function" {
		t.Fatalf("assistant tool call message: %+v", wire[2])
	}
	for _, m := range encodeMessages("openai/gpt-6-luna", messages) {
		if hasBreakpoint(m) {
			t.Fatal("OpenAI models cache by themselves")
		}
	}
}

func TestImagesBecomeParts(t *testing.T) {
	wire := encodeMessages("anthropic/x", []llm.Message{{Role: "user", Text: "Photos", Images: []string{"data:image/png;base64,AA"}}})
	parts := wire[0].Content.([]part)
	if len(parts) != 2 || parts[1].ImageURL["url"] != "data:image/png;base64,AA" || parts[1].CacheControl == nil {
		t.Fatalf("parts %+v", parts)
	}
}

func hasBreakpoint(m message) bool {
	parts, ok := m.Content.([]part)
	return ok && parts[len(parts)-1].CacheControl != nil
}

func TestChatDecodesRecordsUsageAndRetriesOnce(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, "overloaded", http.StatusBadGateway)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		_ = json.Unmarshal(body, &req)
		if req["model"] != "anthropic/claude-haiku-5.5" || r.Header.Get("Authorization") != "Bearer key" || !strings.Contains(string(body), `"effort":"high"`) {
			t.Errorf("request %s", body)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"1","type":"function","function":{"name":"web_search","arguments":"{\"query\":\"x\"}"}}],"reasoning_details":[{"t":1}]}}],
			"usage":{"prompt_tokens":100,"completion_tokens":5,"cost":0.01,"prompt_tokens_details":{"cached_tokens":80}}}`))
	}))
	defer server.Close()

	log := &usageLog{}
	c := &Client{Key: "key", Usage: log, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: server.URL}
	retryPauseForTest(t)
	m, err := c.Chat(llm.For(context.Background(), 7, llm.PurposeChat), llm.Request{Model: llm.Dasyra, Messages: []llm.Message{llm.User("hi")}})
	if err != nil || len(m.ToolCalls) != 1 || m.ToolCalls[0].Name != "web_search" || string(m.Reasoning) != `[{"t":1}]` {
		t.Fatalf("message %+v %v", m, err)
	}
	if calls != 2 || len(log.records) != 1 || log.records[0].UserID != 7 || log.records[0].CachedTokens != 80 || log.records[0].Purpose != "chat" {
		t.Fatalf("calls %d, usage %+v", calls, log.records)
	}
}

func TestChatWithoutChoicesFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer server.Close()
	retryPauseForTest(t)
	c := &Client{Usage: &usageLog{}, Log: slog.New(slog.NewTextHandler(io.Discard, nil)), BaseURL: server.URL}
	if _, err := c.Chat(context.Background(), llm.Request{Model: llm.TextReader}); err == nil {
		t.Fatal("a response without a message must fail")
	}
}

func retryPauseForTest(t *testing.T) {
	old := retryPause
	retryPause = 0
	t.Cleanup(func() { retryPause = old })
}
