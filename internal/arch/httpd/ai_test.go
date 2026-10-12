package httpd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"dasyra/internal/dom"
)

// checkAI accepts only the key "good"
func checkAI(_ context.Context, p dom.AIProvider) error {
	if p.Key != "good" {
		return errors.New("HTTP 401 Unauthorized")
	}
	return nil
}

func suggestPrices(_ context.Context, models []string) (map[string]dom.AIPrice, error) {
	out := map[string]dom.AIPrice{}
	for _, m := range models {
		if m == "glm-5.2" {
			out[m] = dom.AIPrice{Model: m, Input: 0.6, CacheRead: 0.11, Output: 2.2}
		}
	}
	return out, nil
}

func TestPricesOfRolesOffOpenRouter(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/ai")
	token := csrf(t, page)
	post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {token}, "name": {"GLM"}, "kind": {"openai"}, "base_url": {"https://api.z.ai/api/paas/v4"}, "key": {"k"}})
	post(t, c, s.URL+"/panel/ai/roles", url.Values{"csrf": {token}, "provider_dasyra": {"1"}, "model_dasyra": {"glm-5.2"},
		"provider_reflection": {"1"}, "model_reflection": {"glm-5.2"}, "provider_transcription": {"1"}, "model_transcription": {"whisper-1"}})

	_, body := post(t, c, s.URL+"/panel/ai/prices/suggest", url.Values{"csrf": {token}})
	if strings.Count(body, `name="price_`) != 9 || !strings.Contains(body, "Found in the OpenRouter catalog: 1 of 1") || !strings.Contains(body, `name="price_0_input" value="0.6"`) {
		t.Fatalf("one row per model, whisper per minute: %s", body)
	}

	status, body := post(t, c, s.URL+"/panel/ai/prices", url.Values{"csrf": {token}, "price_0_provider": {"1"}, "price_0_model": {"glm-5.2"}, "price_0_input": {"abc"}})
	if status != http.StatusBadRequest || !strings.Contains(body, "a price must be a number") {
		t.Fatalf("bad price: %d", status)
	}
	_, body = post(t, c, s.URL+"/panel/ai/prices", url.Values{"csrf": {token}, "price_0_provider": {"1"}, "price_0_model": {"glm-5.2"}, "price_0_input": {"0,6"},
		"price_0_output": {"2.2"}, "price_1_provider": {"1"}, "price_1_model": {"whisper-1"}, "price_1_per_minute": {"0.006"}})
	if !strings.Contains(body, "Prices saved") || !strings.Contains(body, `name="price_1_per_minute" value="0.006"`) || strings.Contains(body, "no price set") {
		t.Fatalf("saved prices: %s", body)
	}
	// An emptied row drops its price
	_, body = post(t, c, s.URL+"/panel/ai/prices", url.Values{"csrf": {token}, "price_0_provider": {"1"}, "price_0_model": {"glm-5.2"}})
	if strings.Count(body, "no price set") != 1 {
		t.Fatalf("emptied price: %s", body)
	}
}

func TestProvidersKeepKeysOffThePage(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/ai")
	token := csrf(t, page)

	status, body := post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {token}, "name": {"GLM"}, "kind": {"openai"}, "key": {"sk-secret-1234"}})
	if status != http.StatusBadRequest || !strings.Contains(body, "needs a base URL") || !strings.Contains(body, `value="GLM"`) {
		t.Fatalf("missing base URL: %d", status)
	}
	_, body = post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {token}, "name": {"GLM"}, "kind": {"openai"},
		"base_url": {"https://api.z.ai/api/paas/v4/"}, "key": {"sk-secret-1234"}})
	if !strings.Contains(body, "Provider saved.") || !strings.Contains(body, "••••1234") || strings.Contains(body, "sk-secret") {
		t.Fatalf("saved provider page: %s", body)
	}

	// A blank key keeps the saved one, the check gets it too
	_, body = post(t, c, s.URL+"/panel/ai/providers/check", url.Values{"csrf": {token}, "id": {"1"}, "name": {"GLM 2"}, "kind": {"openai"}, "base_url": {"https://api.z.ai/api/paas/v4"}})
	if !strings.Contains(body, "GLM 2: no answer: HTTP 401") {
		t.Fatalf("check with the saved key: %s", body)
	}
	_, body = post(t, c, s.URL+"/panel/ai/providers/check", url.Values{"csrf": {token}, "id": {"1"}, "name": {"GLM"}, "kind": {"openai"}, "base_url": {"https://api.z.ai/api/paas/v4"}, "key": {"good"}})
	if !strings.Contains(body, "GLM: answers /models") || !strings.Contains(body, "••••1234") {
		t.Fatalf("a check must not save: %s", body)
	}
	if status, _ := post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {"forged"}, "name": {"X"}, "kind": {"openrouter"}, "key": {"k"}}); status != http.StatusForbidden {
		t.Fatalf("forged csrf: %d", status)
	}
}

func TestRolesNeedAModelOffOpenRouterAndResetOnDelete(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/ai")
	token := csrf(t, page)
	post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {token}, "name": {"Ollama"}, "kind": {"openai"}, "base_url": {"http://ollama:11434/v1"}})

	roles := url.Values{"csrf": {token}, "provider_dasyra": {"1"}, "provider_reflection": {"0"}, "model_reflection": {"openai/gpt-6-luna"}, "effort_reflection": {"low"}}
	status, body := post(t, c, s.URL+"/panel/ai/roles", roles)
	if status != http.StatusBadRequest || !strings.Contains(body, "Dasyra: set a model for the provider Ollama.") {
		t.Fatalf("role without a model: %d", status)
	}
	roles.Set("model_dasyra", "qwen3:32b")
	if _, body = post(t, c, s.URL+"/panel/ai/roles", roles); !strings.Contains(body, "Roles saved") || !strings.Contains(body, `value="qwen3:32b"`) ||
		!strings.Contains(body, "roles: Dasyra") || !strings.Contains(body, `<option value="low" selected>`) {
		t.Fatalf("saved roles: %s", body)
	}

	if _, body = post(t, c, s.URL+"/panel/ai/providers/delete", url.Values{"csrf": {token}, "id": {"1"}}); !strings.Contains(body, "Provider deleted") || strings.Contains(body, "qwen3:32b") {
		t.Fatalf("delete must reset its roles: %s", body)
	}
	if !strings.Contains(body, `value="openai/gpt-6-luna"`) {
		t.Fatal("other roles stay")
	}
}
