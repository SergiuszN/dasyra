package sqlite

import (
	"testing"
	"time"

	"dasyra/internal/dom"
)

func TestReportTotalsGroupsAndUsers(t *testing.T) {
	db := openTest(t)
	must(t, Users{db}.Update(ctx, &dom.User{ID: dom.OwnerID, Name: "Alex", Timezone: "Europe/Warsaw"}))
	log, report := UsageLog{db}, Report{db}
	must(t, log.Record(ctx, dom.Usage{UserID: 1, Purpose: "chat", Model: "anthropic/claude-haiku-5.5", PromptTokens: 1000, CachedTokens: 800, CompletionTokens: 50, Cost: 0.01}))
	must(t, log.Record(ctx, dom.Usage{UserID: 1, Purpose: "text_reader", Model: "openai/gpt-6-luna", PromptTokens: 500, Cost: 0.002}))
	must(t, log.Record(ctx, dom.Usage{Purpose: "other", Model: "openai/gpt-6-luna", Cost: 0.03}))

	all, err := report.Totals(ctx, nil, nil)
	if err != nil || all.Calls != 3 || all.PromptTokens != 1500 || all.LastAt == "" {
		t.Fatalf("totals %+v %v", all, err)
	}
	nobody := int64(0)
	if none, _ := report.Totals(ctx, nil, &nobody); none.Calls != 1 {
		t.Fatalf("calls without a user %+v", none)
	}
	owner := int64(1)
	models, _ := report.Grouped(ctx, "model", nil, &owner)
	if len(models) != 2 || models[0].Name != "anthropic/claude-haiku-5.5" {
		t.Fatalf("models %+v", models)
	}
	users, _ := report.ByUser(ctx, nil)
	if len(users) != 2 || users[0].UserID != 0 || users[1].Name != "Alex" {
		t.Fatalf("by user %+v", users)
	}
	tomorrow := time.Now().AddDate(0, 0, 1)
	if later, _ := report.Totals(ctx, &tomorrow, nil); later.Calls != 0 {
		t.Fatal("from must filter")
	}
}

func TestReportDailyRepliesAndFirstAt(t *testing.T) {
	db := openTest(t)
	report := Report{db}
	if first, err := report.FirstAt(ctx); err != nil || !first.IsZero() {
		t.Fatalf("empty first %v %v", first, err)
	}
	must(t, UsageLog{db}.Record(ctx, dom.Usage{UserID: 1, Purpose: "chat", Model: "m", Cost: 0.5}))
	must(t, Messages{db}.Insert(ctx, 1, message(1, dom.RoleAssistant, "hi", time.Now())))

	today := time.Now().In(warsaw).Format("2006-01-02")
	daily, _ := report.Daily(ctx, nil, nil)
	if daily[today].Cost != 0.5 {
		t.Fatalf("daily %+v", daily)
	}
	replies, _ := report.RepliesByUser(ctx, time.Now().Add(-time.Hour))
	if replies[1] != 1 {
		t.Fatalf("replies %+v", replies)
	}
	if first, _ := report.FirstAt(ctx); first.IsZero() {
		t.Fatal("first call time")
	}
}
