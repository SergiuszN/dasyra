package llmapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

func TestProbabilityAsksTheDecisionsAPI(t *testing.T) {
	var path string
	e := replying(t, `{"answers":{"yes":{"type":"noul","noul":0.8607}},"usage":{"input_tokens":289,"output_tokens":0,"cost":0.0000058}}`)
	e.Config.Handler = recordPath(&path, e.Config.Handler)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	usage := &usageLog{}
	r := &Router{Log: log, Settings: settings{providers: map[int64]dom.AIProvider{1: {ID: 1, Name: "GLM", Kind: dom.ProviderOpenAI, BaseURL: e.URL}}},
		Default: &Client{Key: "k", BaseURL: e.URL + "/api/v1", Usage: usage, Log: log}}
	ctx := llm.For(context.Background(), 7, llm.PurposeCheckInGate)

	p, err := r.Probability(ctx, llm.CheckInGate, []string{"Now: Mon 15:00"}, "She should write now.")
	if err != nil || p != 0.8607 || path != "/api/alpha/decisions" {
		t.Fatalf("p %v err %v path %s", p, err, path)
	}
	q := e.body["questions"].(map[string]any)["yes"].(map[string]any)
	if e.body["model"] != llm.CheckInGate.ID || q["type"] != "noul" || q["instructions"] != "She should write now." || e.body["state"].([]any)[0] != "Now: Mon 15:00" {
		t.Fatalf("request %v", e.body)
	}
	if u := usage.records[0]; u.UserID != 7 || u.Purpose != llm.PurposeCheckInGate || u.PromptTokens != 289 || u.Cost != 0.0000058 {
		t.Fatalf("usage %+v", u)
	}

	r.Settings = settings{providers: r.Settings.(settings).providers, roles: map[string]dom.AIRole{llm.RoleCheckInGate: {Role: llm.RoleCheckInGate, ProviderID: 1, Model: "x"}}}
	if _, err := r.Probability(ctx, llm.CheckInGate, nil, "q"); err == nil {
		t.Fatal("an OpenAI-compatible provider has no decisions")
	}
}

func recordPath(path *string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*path = r.URL.Path
		next.ServeHTTP(w, r)
	})
}
