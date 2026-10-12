package llmapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type settings struct {
	providers map[int64]dom.AIProvider
	roles     map[string]dom.AIRole
	prices    []dom.AIPrice
}

func (s settings) Providers(context.Context) ([]dom.AIProvider, error) { return nil, nil }
func (s settings) Provider(_ context.Context, id int64) (*dom.AIProvider, error) {
	if p, ok := s.providers[id]; ok {
		return &p, nil
	}
	return nil, nil
}
func (s settings) SaveProvider(context.Context, *dom.AIProvider) error  { return nil }
func (s settings) DeleteProvider(context.Context, int64) error          { return nil }
func (s settings) Roles(context.Context) (map[string]dom.AIRole, error) { return s.roles, nil }
func (s settings) SaveRole(context.Context, dom.AIRole) error           { return nil }
func (s settings) Prices(context.Context) ([]dom.AIPrice, error)        { return s.prices, nil }
func (s settings) SavePrice(context.Context, dom.AIPrice) error         { return nil }
func (s settings) DeletePrice(context.Context, int64, string) error     { return nil }
func (s settings) Price(_ context.Context, id int64, model string) (*dom.AIPrice, error) {
	for _, p := range s.prices {
		if p.ProviderID == id && p.Model == model {
			return &p, nil
		}
	}
	return nil, nil
}

// endpoint answers every chat call and keeps the last request body with its key.
type endpoint struct {
	*httptest.Server
	body map[string]any
	auth string
}

func newEndpoint(t *testing.T) *endpoint {
	return replying(t, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":3,"completion_tokens":1}}`)
}

func replying(t *testing.T, reply string) *endpoint {
	e := &endpoint{}
	e.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		e.body, e.auth = map[string]any{}, r.Header.Get("Authorization")
		_ = json.Unmarshal(raw, &e.body)
		_, _ = w.Write([]byte(reply))
	}))
	t.Cleanup(e.Close)
	return e
}

func TestRouterSendsARoleToItsProviderInPlainOpenAIFormat(t *testing.T) {
	openRouter, glm := newEndpoint(t), newEndpoint(t)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	usage := &usageLog{}
	r := &Router{Log: log, Default: &Client{Key: "env-key", BaseURL: openRouter.URL, Usage: usage, Log: log},
		Settings: settings{
			providers: map[int64]dom.AIProvider{1: {ID: 1, Name: "GLM", Kind: dom.ProviderOpenAI, BaseURL: glm.URL + "/", Key: "glm-key"}},
			roles:     map[string]dom.AIRole{llm.RoleDasyra: {Role: llm.RoleDasyra, ProviderID: 1, Model: "glm-5.2"}, llm.RoleTextReader: {Role: llm.RoleTextReader, Effort: "medium"}},
		}}
	history := []llm.Message{llm.User("hi"), {Role: "assistant", Text: "hello", Reasoning: json.RawMessage(`[{"t":1}]`)}, llm.User("again")}

	if m, err := r.Chat(context.Background(), llm.Request{Model: llm.Dasyra, Messages: history}); err != nil || m.Text != "ok" {
		t.Fatalf("chat %+v %v", m, err)
	}
	b := glm.body
	if b["model"] != "glm-5.2" || b["reasoning_effort"] != "high" || glm.auth != "Bearer glm-key" || b["provider"] != nil || b["reasoning"] != nil || b["max_tokens"] != nil {
		t.Fatalf("GLM request %v %s", b, glm.auth)
	}
	if msgs := b["messages"].([]any); msgs[1].(map[string]any)["reasoning_details"] != nil {
		t.Fatal("reasoning_details are OpenRouter's")
	}
	if usage.records[0].Model != "glm-5.2" || usage.records[0].Cost != 0 {
		t.Fatalf("usage %+v", usage.records)
	}

	// Only the effort set: the default model through OpenRouter, with its extras
	if _, err := r.Chat(context.Background(), llm.Request{Model: llm.TextReader, Messages: history}); err != nil {
		t.Fatal(err)
	}
	if b = openRouter.body; b["model"] != llm.TextReader.ID || b["reasoning"].(map[string]any)["effort"] != "medium" || openRouter.auth != "Bearer env-key" {
		t.Fatalf("OpenRouter request %v", b)
	}
}

func TestOpenAIKindWithoutReasoningSendsNoEffort(t *testing.T) {
	r := encode(llm.Request{Model: llm.Model{ID: "llama3", Effort: "none"}}, false)
	if r.ReasoningEffort != "" || r.Reasoning != nil {
		t.Fatalf("request %+v", r)
	}
}

func TestCheckAsksOpenRouterForTheKey(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer good" {
			http.Error(w, "no", http.StatusUnauthorized)
		}
	}))
	defer server.Close()
	r := &Router{Default: &Client{}}
	if err := r.Check(context.Background(), dom.AIProvider{Kind: dom.ProviderOpenRouter, BaseURL: server.URL, Key: "bad"}); err == nil {
		t.Fatal("a wrong key must fail")
	}
	if err := r.Check(context.Background(), dom.AIProvider{Kind: dom.ProviderOpenAI, BaseURL: server.URL, Key: "good"}); err != nil {
		t.Fatal(err)
	}
	if paths[0] != "/key" || paths[1] != "/models" {
		t.Fatalf("paths %v", paths)
	}
}

func TestMissingCostIsCountedFromThePrice(t *testing.T) {
	// DeepSeek's cache fields, no cost
	deepseek := replying(t, `{"choices":[{"message":{"content":"ok"}}],"usage":{"prompt_tokens":1000000,"completion_tokens":100000,"prompt_cache_hit_tokens":800000}}`)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	usage := &usageLog{}
	s := settings{
		providers: map[int64]dom.AIProvider{2: {ID: 2, Name: "DeepSeek", Kind: dom.ProviderOpenAI, BaseURL: deepseek.URL}},
		roles: map[string]dom.AIRole{llm.RoleDasyra: {Role: llm.RoleDasyra, ProviderID: 2, Model: "deepseek-chat"},
			llm.RoleReflection: {Role: llm.RoleReflection, ProviderID: 2, Model: "deepseek-reasoner"}},
		prices: []dom.AIPrice{{ProviderID: 2, Model: "deepseek-chat", Input: 0.5, CacheRead: 0.05, Output: 2}},
	}
	r := &Router{Settings: s, Log: log, Default: &Client{Usage: usage, Log: log}}
	for _, m := range []llm.Model{llm.Dasyra, llm.Reflection} {
		if _, err := r.Chat(context.Background(), llm.Request{Model: m, Messages: []llm.Message{llm.User("hi")}}); err != nil {
			t.Fatal(err)
		}
	}
	priced, unpriced := usage.records[0], usage.records[1]
	if priced.CachedTokens != 800000 || !priced.Estimated || math.Abs(priced.Cost-(0.1+0.04+0.2)) > 1e-9 {
		t.Fatalf("priced %+v", priced)
	}
	if unpriced.Estimated || unpriced.Cost != 0 {
		t.Fatalf("a model without a price stays at 0: %+v", unpriced)
	}
}

func TestSuggestPricesFindsModelsByName(t *testing.T) {
	catalog := replying(t, `{"data":[{"id":"z-ai/glm-5.2","pricing":{"prompt":"0.0000006","completion":"0.0000022","input_cache_read":"0.00000011"}},
		{"id":"openai/gpt-6","pricing":{"prompt":"0.00000125","completion":"0.00001"}},{"id":"gpt-6","pricing":{"prompt":"0.000009","completion":"0"}}]}`)
	r := &Router{Default: &Client{BaseURL: catalog.URL}}
	found, err := r.SuggestPrices(context.Background(), []string{"glm-5.2", "openai/gpt-6", "llama3"})
	if err != nil || len(found) != 2 {
		t.Fatalf("found %+v %v", found, err)
	}
	if g := found["glm-5.2"]; g.Input != 0.6 || g.Output != 2.2 || g.CacheRead != 0.11 || found["openai/gpt-6"].Input != 1.25 {
		t.Fatalf("prices %+v", found)
	}
}
