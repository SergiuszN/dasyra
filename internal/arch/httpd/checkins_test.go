package httpd

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestCheckInsPageSavesThePlan(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/check-ins")
	if strings.Count(page, `name="hour" value=`) != 24 || strings.Count(page, " checked>") != 5 || !strings.Contains(page, "now 15:00") ||
		!strings.Contains(page, "perplexity/pplx-decider-v1.1-27b") {
		t.Fatalf("default plan: %s", page)
	}
	token := csrf(t, page)

	status, body := post(t, c, s.URL+"/panel/check-ins", url.Values{"csrf": {token}, "hour": {"10"}, "quiet_days": {"two"}, "stop_days": {"5"}, "threshold": {"0.3"}})
	if status != http.StatusBadRequest || !strings.Contains(body, "whole numbers") {
		t.Fatalf("bad days: %d", status)
	}
	status, _ = post(t, c, s.URL+"/panel/check-ins", url.Values{"csrf": {token}, "hour": {"10"}, "quiet_days": {"2"}, "stop_days": {"5"}, "threshold": {"2"}})
	if status != http.StatusBadRequest {
		t.Fatal("a threshold over 1")
	}
	_, body = post(t, c, s.URL+"/panel/check-ins", url.Values{"csrf": {token}, "hour": {"20", "8"}, "quiet_days": {"3"}, "stop_days": {"0"}, "gate": {"on"}, "threshold": {"0,25"}})
	if !strings.Contains(body, "Check-ins saved") || strings.Count(body, " checked") != 3 || !strings.Contains(body, `name="threshold" value="0.25"`) ||
		!strings.Contains(body, "now 20:00") {
		t.Fatalf("saved plan: %s", body)
	}

	// The gate model can't move to an OpenAI-compatible provider
	_, page = get(t, c, s.URL+"/panel/ai")
	post(t, c, s.URL+"/panel/ai/providers", url.Values{"csrf": {token}, "name": {"GLM"}, "kind": {"openai"}, "base_url": {"https://api.z.ai/api/paas/v4"}, "key": {"k"}})
	status, body = post(t, c, s.URL+"/panel/ai/roles", url.Values{"csrf": {token}, "provider_check_in_gate": {"1"}, "model_check_in_gate": {"glm-5.2"}})
	if status != http.StatusBadRequest || !strings.Contains(body, "only OpenRouter serves decision models") {
		t.Fatalf("gate on GLM: %d", status)
	}
}
