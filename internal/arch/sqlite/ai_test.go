package sqlite

import (
	"testing"

	"dasyra/internal/dom"
)

func TestAISettingsRoundTrip(t *testing.T) {
	ai := AISettings{openTest(t)}
	p := &dom.AIProvider{Name: "GLM", Kind: dom.ProviderOpenAI, BaseURL: "https://api.z.ai/api/paas/v4", Key: "k1"}
	must(t, ai.SaveProvider(ctx, p))
	p.Key = "k2"
	must(t, ai.SaveProvider(ctx, p))
	if got, _ := ai.Provider(ctx, p.ID); got == nil || got.Key != "k2" || got.Name != "GLM" {
		t.Fatalf("provider %+v", got)
	}

	must(t, ai.SaveRole(ctx, dom.AIRole{Role: "dasyra", ProviderID: p.ID, Model: " glm-5.2 "}))
	must(t, ai.SaveRole(ctx, dom.AIRole{Role: "reflection", Effort: "low"}))
	must(t, ai.SaveRole(ctx, dom.AIRole{Role: "text_reader", Model: "x"}))
	must(t, ai.SaveRole(ctx, dom.AIRole{Role: "text_reader"})) // back to defaults
	roles, err := ai.Roles(ctx)
	if err != nil || len(roles) != 2 || roles["dasyra"].Model != "glm-5.2" || roles["dasyra"].ProviderID != p.ID || roles["reflection"].ProviderID != 0 {
		t.Fatalf("roles %+v %v", roles, err)
	}

	must(t, ai.DeleteProvider(ctx, p.ID))
	roles, _ = ai.Roles(ctx)
	if gone, _ := ai.Provider(ctx, p.ID); gone != nil || len(roles) != 1 || roles["reflection"].Effort != "low" {
		t.Fatalf("after delete %+v %+v", gone, roles)
	}
}

func TestPricesAndEstimatedUsage(t *testing.T) {
	db := openTest(t)
	ai := AISettings{db}
	p := &dom.AIProvider{Name: "GLM", Kind: dom.ProviderOpenAI, BaseURL: "https://x"}
	must(t, ai.SaveProvider(ctx, p))
	must(t, ai.SavePrice(ctx, dom.AIPrice{ProviderID: p.ID, Model: "glm-5.2", Input: 1}))
	must(t, ai.SavePrice(ctx, dom.AIPrice{ProviderID: p.ID, Model: "glm-5.2", Input: 0.6, Output: 2.2}))
	if got, _ := ai.Price(ctx, p.ID, "glm-5.2"); got == nil || got.Input != 0.6 || got.Output != 2.2 {
		t.Fatalf("price %+v", got)
	}

	log := UsageLog{db}
	must(t, log.Record(ctx, dom.Usage{Purpose: "chat", Model: "glm-5.2", PromptTokens: 10, Cost: 0.001, Estimated: true}))
	must(t, log.Record(ctx, dom.Usage{Purpose: "chat", Model: "llama3", PromptTokens: 10}))
	must(t, log.Record(ctx, dom.Usage{Purpose: "chat", Model: "anthropic/x", PromptTokens: 10, Cost: 0.002}))
	totals, err := Report{db}.Totals(ctx, nil, nil)
	if err != nil || totals.Estimated != 1 || totals.Unpriced != 1 || totals.Calls != 3 {
		t.Fatalf("totals %+v %v", totals, err)
	}

	must(t, ai.DeleteProvider(ctx, p.ID))
	if prices, _ := ai.Prices(ctx); len(prices) != 0 {
		t.Fatalf("prices of a deleted provider stay: %+v", prices)
	}
}
