package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type benchScript struct {
	replies  []llm.Message
	requests []llm.Request
	payers   []llm.Payer
}

func (s *benchScript) Chat(ctx context.Context, req llm.Request) (llm.Message, error) {
	s.requests = append(s.requests, req)
	s.payers = append(s.payers, llm.PayerOf(ctx))
	answer := s.replies[0]
	if len(s.replies) > 1 {
		s.replies = s.replies[1:]
	}
	return answer, nil
}

func benchFixture() benchCase {
	return benchCase{ID: "example", Title: "Example", Mode: "chat", At: "2026-11-04 10:00:00",
		User:    benchUser{Name: "Alex", Profile: "Respond in English.", Timezone: "Europe/London"},
		Message: "The parcel is collected.", Review: []string{"PRIVATE_REVIEW_NOT_FOR_MODEL"}, Source: "PRIVATE_SOURCE_NOT_FOR_MODEL",
		Todos: []benchTodo{{ID: 301, Text: "Collect parcel", Day: "2026-11-04"}}}
}

func benchTestOptions() benchOptions {
	return benchOptions{Timeout: time.Minute, NoEmbeddings: true}
}

func benchToolReply(name, args string) llm.Message {
	return llm.Message{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call1", Name: name, Arguments: args}}}
}

func TestBenchRunsRealToolsAndStartsEachCaseFresh(t *testing.T) {
	c := benchFixture()
	for i := 0; i < 2; i++ {
		script := &benchScript{replies: []llm.Message{
			benchToolReply("save_todos", `{"items":[{"id":301,"status":"done"}]}`),
			{Role: "assistant", Text: "REACTION: 👍"},
		}}
		r := runBenchCase(context.Background(), c, benchTestOptions(), filepath.Join(t.TempDir(), "case.sqlite"), "", script)
		if r.Error != "" || len(r.Calls) != 1 || len(r.Reactions) != 1 || len(r.Sent) != 0 {
			t.Fatalf("result %+v", r)
		}
		if r.Before["todos"][0]["done_at"] != nil || r.After["todos"][0]["done_at"] == nil {
			t.Fatalf("before %v after %v", r.Before, r.After)
		}
		if script.payers[0].UserID != dom.OwnerID || script.payers[0].Purpose != llm.PurposeChat {
			t.Fatalf("payer %+v", script.payers)
		}
		data, _ := json.Marshal(script.requests)
		if strings.Contains(string(data), "PRIVATE_") || !strings.Contains(string(data), "2026-11-04 Wed 10:00") || len(script.requests[0].Tools) == 0 {
			t.Fatalf("wrong prompt %s", data)
		}
	}
}

func TestBenchCapturesUnsentToolMutations(t *testing.T) {
	c := benchFixture()
	c.Mode, c.Message = "checkin", ""
	c.History = []benchMessage{{Role: dom.RoleUser, Text: "I will collect the parcel."}, {Role: dom.RoleAssistant, Text: "Okay."}}
	script := &benchScript{replies: []llm.Message{
		benchToolReply("save_todos", `{"items":[{"id":301,"status":"done"}]}`),
		{Role: "assistant", Text: "NO_MESSAGE"},
	}}
	r := runBenchCase(context.Background(), c, benchTestOptions(), filepath.Join(t.TempDir(), "case.sqlite"), "", script)
	if r.Error != "" || len(r.Sent) != 0 || len(r.Calls) != 1 || r.After["todos"][0]["done_at"] == nil || r.Steps[1].Reply.Text != "NO_MESSAGE" {
		t.Fatalf("result %+v", r)
	}
}

func TestBenchRoleSettingsAndScheduledTrigger(t *testing.T) {
	c := benchFixture()
	c.Mode, c.Message = "reminder", ""
	c.Events = []benchEvent{{ID: 101, Title: "Collect parcel", Start: c.At}}
	opts := benchTestOptions()
	opts.Roles = map[string]benchRole{llm.RoleReminder: {Model: "test/reminder", Effort: "xhigh"}, llm.RoleDasyra: {Model: "test/chat", Effort: "low"}}
	script := &benchScript{replies: []llm.Message{{Role: "assistant", Text: "Collect the parcel now."}}}
	r := runBenchCase(context.Background(), c, opts, filepath.Join(t.TempDir(), "case.sqlite"), "", script)
	if r.Error != "" || len(script.requests) != 1 || script.requests[0].Model.ID != "test/reminder" || script.requests[0].Model.Effort != "xhigh" {
		t.Fatalf("wrong role: %+v", r)
	}
	data, _ := json.Marshal(script.requests[0].Messages)
	if !strings.Contains(string(data), "EVENT DUE NOW") || r.Before["events"][0]["reminded_at"] != nil || r.After["events"][0]["reminded_at"] == nil {
		t.Fatalf("missing real reminder trigger %s / %v", data, r.After)
	}
}

func TestBenchReflectionRunsItsOwnToolsAndRole(t *testing.T) {
	c := benchFixture()
	c.Mode, c.Message = "reflection", ""
	c.History = []benchMessage{{Role: dom.RoleUser, Text: "I collected the parcel."}}
	opts := benchTestOptions()
	opts.Roles = map[string]benchRole{llm.RoleReflection: {Model: "test/reflect", Effort: "high"}}
	script := &benchScript{replies: []llm.Message{
		benchToolReply("create_memory", `{"name":"Journal 2026-11-04","category":"journal","content":"Collected the parcel."}`),
		{Role: "assistant", Text: "DONE"},
	}}
	r := runBenchCase(context.Background(), c, opts, filepath.Join(t.TempDir(), "case.sqlite"), "", script)
	if r.Error != "" || len(r.Sent) != 0 || len(r.After["memories"]) != 1 || r.Steps[0].Request.Model.ID != "test/reflect" {
		t.Fatalf("reflection %+v", r)
	}
	for _, spec := range r.Steps[0].Request.Tools {
		if externalBenchTool(spec.Name) {
			t.Fatal("reflection got chat tools")
		}
	}
}

func TestBenchFixedExternalResultsAndReasoningCapture(t *testing.T) {
	c := benchFixture()
	c.Mocks = []benchMock{{Tool: "web_search", Match: map[string]string{"query": "parcel"}, Result: "Fixed search result"}}
	first := benchToolReply("web_search", `{"query":"parcel"}`)
	first.Reasoning = json.RawMessage(`[{"type":"reasoning.text","text":"live-loop-only"}]`)
	script := &benchScript{replies: []llm.Message{first, {Role: "assistant", Text: "Here is the result."}}}
	r := runBenchCase(context.Background(), c, benchTestOptions(), filepath.Join(t.TempDir(), "case.sqlite"), "", script)
	if r.Error != "" || r.Calls[0].Result != "Fixed search result" {
		t.Fatalf("mocks %+v", r)
	}
	live, _ := json.Marshal(script.requests[1])
	saved, _ := json.Marshal(r.Steps)
	if !strings.Contains(string(live), "live-loop-only") || strings.Contains(string(saved), "live-loop-only") {
		t.Fatal("reasoning was lost in the live loop or leaked into the report")
	}
}

func TestBenchReportsRejectedCalls(t *testing.T) {
	script := &benchScript{replies: []llm.Message{
		benchToolReply("invented_tool", `{}`),
		{Role: "assistant", Text: "That tool is unavailable."},
	}}
	r := runBenchCase(context.Background(), benchFixture(), benchTestOptions(), filepath.Join(t.TempDir(), "case.sqlite"), "", script)
	if r.Error != "" || len(r.Calls) != 1 || r.Calls[0].Result != "Unknown tool: invented_tool" {
		t.Fatalf("rejected call missing: %+v", r)
	}
}

func TestBenchCaseSelectionValidationAndNeverOpensDBPath(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cases.json")
	c := benchFixture()
	writeBenchJSON(file, []benchCase{c})
	if _, err := readBenchCases(file, "ex*"); err != nil {
		t.Fatal(err)
	}
	if _, err := readBenchCases(file, "missing"); err == nil {
		t.Fatal("unknown selection accepted")
	}
	writeBenchJSON(file, []benchCase{c, c})
	if _, err := readBenchCases(file, ""); err == nil {
		t.Fatal("duplicate ids accepted")
	}
	c.History = []benchMessage{{Role: dom.RoleUser, Text: "future", At: "2026-11-05 10:00:00"}}
	writeBenchJSON(file, []benchCase{c})
	if _, err := readBenchCases(file, ""); err == nil {
		t.Fatal("future history accepted")
	}
	c = benchFixture()
	writeBenchJSON(file, []benchCase{c})
	sentinel := filepath.Join(dir, "prod.sqlite")
	os.WriteFile(sentinel, []byte("do not open"), 0600)
	t.Setenv("DB_PATH", sentinel)
	t.Setenv("SERVER_TZ", "not-a-zone")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := bench(log, []string{"-cases", file, "-dry-run", "-out", filepath.Join(dir, "run")}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(sentinel)
	if string(data) != "do not open" {
		t.Fatal("DB_PATH changed")
	}
	if err := bench(log, []string{"-cases", file, "-dry-run", "-out", filepath.Join(dir, "run")}); err == nil {
		t.Fatal("existing output directory overwritten")
	}
}
