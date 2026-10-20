package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

func (w *world) checkIns() *CheckIns {
	zone, _ := time.LoadLocation("Europe/Warsaw")
	return &CheckIns{Users: sqlite.Users{DB: w.db}, Messages: w.messages, Plans: sqlite.Settings{DB: w.db}, Dasyra: w.dasyra, Zone: zone, Log: discard}
}

func TestCheckInsThinOutForSilentUsers(t *testing.T) {
	w := newWorld(t, answers("Как дела с бизнес-планом?"))
	zone, _ := time.LoadLocation("Europe/Warsaw")
	day := func(d, hour int) time.Time { return time.Date(2026, 3, d, hour, 0, 0, 0, zone) }
	w.say(t, dom.RoleUser, "сегодня сажусь за бизнес-план", day(1, 20))
	c := w.checkIns()

	at(t, day(4, 9)) // three days quiet: only the middle hour, 15
	must(t, c.Scheduled(ctx))
	at(t, day(4, 10)) // not an hour of the plan
	must(t, c.Scheduled(ctx))
	if len(w.ai.requests) != 0 {
		t.Fatal("quiet user before the middle hour")
	}
	at(t, day(4, 15))
	must(t, c.Scheduled(ctx))
	if len(w.chat.sent) != 1 {
		t.Fatalf("sent %q", w.chat.sent)
	}

	w.say(t, dom.RoleUser, "ок", day(4, 16))
	at(t, day(10, 15)) // five days quiet: none at all
	must(t, c.Scheduled(ctx))
	if len(w.chat.sent) != 1 {
		t.Fatal("silent for five days")
	}

	// The plan comes from the panel
	must(t, sqlite.Settings{DB: w.db}.SaveCheckInPlan(ctx, dom.CheckInPlan{Hours: []int{10}}))
	must(t, c.Scheduled(ctx))
	at(t, day(11, 10))
	must(t, c.Scheduled(ctx))
	if len(w.chat.sent) != 2 {
		t.Fatalf("own hours, rules off: sent %q", w.chat.sent)
	}
}

// decider answers every question with p, or fails
type decider struct {
	p      float64
	err    error
	states [][]string
}

func (d *decider) Probability(_ context.Context, _ llm.Model, state []string, _ string) (float64, error) {
	d.states = append(d.states, state)
	return d.p, d.err
}

func TestCheckInGateSkipsEmptyRunsAndFailsOpen(t *testing.T) {
	w := newWorld(t, answers("Как прошёл созвон?"))
	at(t, time.Now())
	w.say(t, dom.RoleUser, "иду на созвон с инвестором", now().Add(-4*time.Hour))
	gate := &decider{p: 0.1}
	c := w.checkIns()
	c.Gate = gate

	must(t, c.Run(ctx))
	if len(gate.states) != 0 || len(w.chat.sent) != 1 {
		t.Fatal("the gate is off by default")
	}

	plan := dom.DefaultCheckInPlan()
	plan.Gate = true
	must(t, sqlite.Settings{DB: w.db}.SaveCheckInPlan(ctx, plan))
	at(t, now().Add(5*time.Hour))
	w.say(t, dom.RoleUser, "созвон прошёл", now().Add(-4*time.Hour))
	must(t, c.Run(ctx))
	state := strings.Join(gate.states[0], "\n")
	if len(w.chat.sent) != 1 || !strings.Contains(state, "North stars") || !strings.Contains(state, "созвон с инвестором") || strings.Contains(state, "PROACTIVE MODE") {
		t.Fatalf("below the threshold: sent %q, state %q", w.chat.sent, state)
	}

	gate.p = 0.6
	must(t, c.Run(ctx))
	gate.err = errors.New("HTTP 503")
	at(t, now().Add(5*time.Hour))
	w.say(t, dom.RoleUser, "ещё одно", now().Add(-4*time.Hour))
	must(t, c.Run(ctx))
	if len(w.chat.sent) != 3 {
		t.Fatalf("above the threshold and a failed gate: sent %q", w.chat.sent)
	}
}

func TestCheckInsSkipRecentTalkAndTwoUnansweredCheckIns(t *testing.T) {
	w := newWorld(t, answers("Как прошёл созвон?"))
	at(t, time.Now())
	w.say(t, dom.RoleUser, "иду на созвон", now().Add(-time.Hour))
	must(t, w.checkIns().Run(ctx))
	if len(w.ai.requests) != 0 {
		t.Fatal("talked within three hours")
	}

	// Cron hours are three hours apart, a check-in is the last message too
	at(t, now().Add(3*time.Hour))
	must(t, w.checkIns().Run(ctx))
	at(t, now().Add(3*time.Hour))
	must(t, w.checkIns().Run(ctx))
	if len(w.chat.sent) != 2 {
		t.Fatalf("sent %q", w.chat.sent)
	}
	at(t, now().Add(3*time.Hour))
	must(t, w.checkIns().Run(ctx))
	if len(w.chat.sent) != 2 {
		t.Fatal("two unanswered check-ins silence the next ones")
	}
}

func TestOnboardingNudgeOnlyOnceAfterTwentyHours(t *testing.T) {
	w := newWorld(t, answers("Как твой день?"))
	w.user.Profile = ""
	must(t, sqlite.Users{DB: w.db}.Update(ctx, w.user))
	at(t, time.Now())
	w.say(t, dom.RoleAssistant, "Привет! Я Дасира. Чем занимаешься?", now().Add(-21*time.Hour))

	must(t, w.checkIns().Run(ctx))
	must(t, w.checkIns().Run(ctx))
	if len(w.chat.sent) != 1 || !strings.Contains(w.ai.requests[0].Messages[0].Text, "send one short, friendly, no-pressure nudge") {
		t.Fatalf("sent %q", w.chat.sent)
	}
}

func TestDueReminderWakesDasyraOnce(t *testing.T) {
	w := newWorld(t, answers("Пора позвонить маме"))
	local := w.user.Local(time.Now())
	must(t, w.events.Insert(ctx, w.user, &dom.Event{ID: 1, Title: "Позвонить маме", Start: local.Add(-2 * time.Minute)}))
	r := &Reminders{Users: sqlite.Users{DB: w.db}, Messages: w.messages, Events: w.events, Dasyra: w.dasyra, Log: discard}

	must(t, r.Run(ctx))
	must(t, r.Run(ctx))
	if fmt.Sprint(w.chat.sent) != "[Пора позвонить маме]" {
		t.Fatalf("sent %q", w.chat.sent)
	}
	trigger := w.ai.requests[0].Messages[len(w.ai.requests[0].Messages)-1].Text
	if !strings.HasPrefix(trigger, "EVENT DUE NOW: #1 Позвонить маме on ") || !strings.Contains(trigger, "Otherwise remind Alex about it") {
		t.Fatalf("trigger %q", trigger)
	}
	if strings.Contains(w.ai.requests[0].Messages[0].Text, "PROACTIVE MODE") {
		t.Fatal("a reminder skips the check-in rules")
	}
}

func TestForgottenMessageIsAnswered(t *testing.T) {
	w := newWorld(t, answers("Прости, отвечаю"))
	w.say(t, dom.RoleUser, "ты тут?", time.Now().Add(-10*time.Minute))
	r := &Reminders{Users: sqlite.Users{DB: w.db}, Messages: w.messages, Events: w.events, Dasyra: w.dasyra, Log: discard}
	must(t, r.Run(ctx))
	if len(w.chat.sent) != 1 {
		t.Fatalf("sent %q", w.chat.sent)
	}
}

func TestReflectionWritesAJournalAndMovesOn(t *testing.T) {
	w := newWorld(t, answers("DONE").thenCall("create_memory", `{"name":"Journal","category":"journal","content":"Купил мотоцикл"}`))
	users := sqlite.Users{DB: w.db}
	memories := sqlite.Memories{DB: w.db}
	last := w.say(t, dom.RoleUser, "купил мотоцикл Honda", time.Now().Add(-time.Hour).Truncate(time.Second))
	r := &Reflection{Users: users, Messages: w.messages, Memories: memories, AI: w.ai, Log: discard,
		Tools: tool.NewRegistry(discard, tool.CreateMemory{Memories: memories, IDs: w.ids})}

	must(t, r.Reflect(ctx, w.user))
	journal, _ := memories.List(ctx, w.user.ID, dom.CategoryJournal, 0, 10)
	reloaded, _ := users.Get(ctx, w.user.ID)
	if len(journal) != 1 || !reloaded.LastReflectedAt.Equal(last.CreatedAt) {
		t.Fatalf("journal %+v reflected %v", journal, reloaded.LastReflectedAt)
	}
	if !strings.Contains(w.ai.requests[0].Messages[0].Text, `named "Journal `) {
		t.Fatal("the journal name is in the prompt")
	}

	must(t, r.Reflect(ctx, reloaded))
	if len(w.ai.requests) != 2 {
		t.Fatal("nothing new since the last reflection")
	}
}

func TestInboxRegistersByInviteAndStoresAttachments(t *testing.T) {
	w := newWorld(t, answers("Привет! Я Дасира."))
	users, invites := sqlite.Users{DB: w.db}, sqlite.Invites{DB: w.db}
	must(t, invites.Create(ctx, "code1", ""))
	in := &Inbox{Users: users, Invites: invites, Messages: w.messages, Dasyra: w.dasyra, Chat: w.chat, Voice: voice{}, IDs: w.ids, Log: discard, Work: NewBackground(ctx)}

	must(t, in.Receive(ctx, Incoming{ChatID: 2000, Private: true, Text: "hi"}))
	must(t, in.Receive(ctx, Incoming{ChatID: 2000, Private: true, FirstName: "Danil", Text: "/start code1"}))
	in.Work.Wait()
	danil, _ := users.ByChat(ctx, 2000)
	if danil == nil || danil.Name != "Danil" || fmt.Sprint(w.chat.sent) != "[Dasyra is invite-only for now. Привет! Я Дасира.]" {
		t.Fatalf("user %+v sent %q", danil, w.chat.sent)
	}

	must(t, in.Receive(ctx, Incoming{ChatID: 2000, MessageID: 7, Private: true, DocumentFileID: "f", DocumentName: "menu.pdf", Text: "что тут?"}))
	must(t, in.Receive(ctx, Incoming{ChatID: 2000, MessageID: 7, Private: true, DocumentFileID: "f", DocumentName: "menu.pdf"}))
	must(t, in.Receive(ctx, Incoming{ChatID: 2000, MessageID: 8, Private: true, AudioFileID: "a", AudioFileName: "voice.ogg"}))
	in.Work.Wait()
	h, _ := w.messages.Last(ctx, danil.ID, 10)
	got := []string{}
	for _, m := range h.OldestFirst() {
		if m.Role == dom.RoleUser {
			got = append(got, m.Content)
		}
	}
	if fmt.Sprint(got) != `[[file "menu.pdf", read_document id 7] что тут? [voice] напомни про молоко]` {
		t.Fatalf("stored %q", got)
	}

	// Turned off in the admin panel: turned away like a stranger, nothing stored
	must(t, users.SetDisabled(ctx, danil.ID, now()))
	w.chat.sent = nil
	must(t, in.Receive(ctx, Incoming{ChatID: 2000, MessageID: 9, Private: true, Text: "ты тут?"}))
	in.Work.Wait()
	if after, _ := w.messages.Last(ctx, danil.ID, 10); len(after) != len(h) || fmt.Sprint(w.chat.sent) != "[Dasyra is invite-only for now.]" {
		t.Fatalf("history %d sent %q", len(after), w.chat.sent)
	}
}

type voice struct{}

func (voice) Transcribe(context.Context, []byte, string) (string, error) {
	return "напомни про молоко", nil
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
