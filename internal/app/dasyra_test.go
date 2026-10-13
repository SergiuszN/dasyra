package app

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

func TestReplyIsSplitIntoPartsAndStored(t *testing.T) {
	w := newWorld(t, answers("Привет!\n\nКак прошла тренировка?"))
	w.say(t, dom.RoleUser, "привет", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})

	if fmt.Sprint(w.chat.sent) != "[Привет! Как прошла тренировка?]" {
		t.Fatalf("sent %q", w.chat.sent)
	}
	if newest := w.history(t).Newest(); newest.Role != dom.RoleAssistant || newest.CheckIn {
		t.Fatalf("stored %+v", newest)
	}
}

func TestPromptOrderGoesFromStableToVolatile(t *testing.T) {
	w := newWorld(t, answers("ok"))
	w.say(t, dom.RoleUser, "что по мотоциклу", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})

	m := w.ai.requests[0].Messages
	if m[0].Role != "system" || !m[0].Cache || !strings.HasPrefix(m[1].Text, "Recent and upcoming events:") || !m[1].Cache ||
		!strings.HasPrefix(m[2].Text, "Current conversation (oldest to newest):") || !m[2].Cache ||
		!strings.HasPrefix(m[len(m)-1].Text, "Current date and time: ") {
		t.Fatalf("prompt %+v", m)
	}
	if !strings.Contains(m[0].Text, "Profile of Alex:") || strings.Contains(m[0].Text, "PROACTIVE MODE") {
		t.Fatal("a reply has the profile and no check-in rules")
	}
}

func TestClosingMessageGetsAReaction(t *testing.T) {
	w := newWorld(t, answers("REACTION: ❤️"))
	w.say(t, dom.RoleUser, "спасибо, побежал", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})

	if len(w.chat.sent) != 0 || fmt.Sprint(w.chat.reactions) != "[❤]" {
		t.Fatalf("sent %q reactions %q", w.chat.sent, w.chat.reactions)
	}
	if w.history(t).Newest().Role != dom.RoleAssistant {
		t.Fatal("a reaction is stored, or the message would be answered again as dropped")
	}
}

func TestCheckInStaysSilentOnNoMessageAndReaction(t *testing.T) {
	for _, answer := range []string{"NO_MESSAGE", "REACTION: 👍", errorText} {
		w := newWorld(t, answers(answer))
		w.say(t, dom.RoleUser, "ок", time.Now().Add(-5*time.Hour))
		w.dasyra.Speak(ctx, w.user, Run{Proactive: true})
		if len(w.chat.sent)+len(w.chat.reactions) != 0 || w.history(t).Newest().Role != dom.RoleUser {
			t.Fatalf("%q: sent %q reactions %q", answer, w.chat.sent, w.chat.reactions)
		}
		if !strings.Contains(w.ai.requests[0].Messages[0].Text, "PROACTIVE MODE (cron-triggered):") {
			t.Fatal("a check-in gets the proactive rules")
		}
	}
}

func TestAlreadyAnsweredMessageIsSkipped(t *testing.T) {
	w := newWorld(t, answers("again"))
	w.say(t, dom.RoleUser, "вопрос", time.Now().Add(-time.Minute))
	w.say(t, dom.RoleAssistant, "ответ", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})
	if len(w.ai.requests) != 0 {
		t.Fatal("the reply that ran meanwhile already answered it")
	}
}

func TestMessageArrivingDuringAReplyGetsAFollowUp(t *testing.T) {
	w := newWorld(t, answers())
	w.say(t, dom.RoleUser, "первый вопрос", time.Now())
	// The second message lands while the model thinks about the first one
	w.dasyra.AI = &interrupting{script: answers("ответ на первый", "ответ на второй"), arrive: func() { w.say(t, dom.RoleUser, "второй вопрос", time.Now()) }}
	w.dasyra.Speak(ctx, w.user, Run{})

	if fmt.Sprint(w.chat.sent) != "[ответ на первый ответ на второй]" {
		t.Fatalf("sent %q", w.chat.sent)
	}
}

// interrupting runs arrive during its first call.
type interrupting struct {
	*script
	arrive func()
}

func (i *interrupting) Chat(c context.Context, r llm.Request) (llm.Message, error) {
	if i.arrive != nil {
		i.arrive()
		i.arrive = nil
	}
	return i.script.Chat(c, r)
}

func TestToolCallsAreStoredAndHintedNextTime(t *testing.T) {
	w := newWorld(t, answers("Солнечно").thenCall("web_search", `{"query":"погода Щецин","mode":""}`))
	w.say(t, dom.RoleUser, "какая погода?", time.Now().Add(-time.Minute))
	w.dasyra.Speak(ctx, w.user, Run{})

	next := answers("Да")
	w.dasyra.AI = next
	w.say(t, dom.RoleUser, "а завтра?", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})
	if !strings.Contains(next.prompt(), `Tool log (not a message from Alex): your reply at`) ||
		!strings.Contains(next.prompt(), `used web_search {"query":"погода Щецин"}. Their full results are in recent_tool_results.`) {
		t.Fatalf("prompt %s", next.prompt())
	}
}

func TestWebCallsGetANudgeAndThenALimit(t *testing.T) {
	w := newWorld(t, answers())
	log := &callLog{}
	exec := w.dasyra.executor(tool.NewSession(w.user), log)
	var last string
	for range webCallsLimit + 1 {
		last = exec(ctx, "web_search", `{"query":"q"}`)
	}
	if log.web != webCallsLimit || !strings.HasPrefix(last, "Not executed: the web limit") {
		t.Fatalf("web %d last %q", log.web, last)
	}
	if !strings.Contains(log.calls[webCallsNudge-1].Result, "[20 web calls in this reply.") || strings.Contains(log.calls[webCallsNudge-2].Result, "web calls in this reply") {
		t.Fatal("the nudge comes from the 20th call")
	}
}

func TestMemoriesFoundByTheShortReplyAndThePreviousMessage(t *testing.T) {
	w := newWorld(t, answers("ok"))
	m := &dom.Memory{ID: w.ids.Next(), Name: "Мотоцикл", Category: "plans", Content: "Хочет купить Honda CB500", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := w.dasyra.Memories.Insert(ctx, w.user.ID, m); err != nil {
		t.Fatal(err)
	}
	w.say(t, dom.RoleAssistant, "Посмотреть объявления про мотоцикл Honda?", time.Now().Add(-time.Minute))
	w.say(t, dom.RoleUser, "да, давай", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})
	if !strings.Contains(w.ai.prompt(), "Relevant memories (use if helpful): [#") || !strings.Contains(w.ai.prompt(), "Хочет купить Honda CB500") {
		t.Fatalf("prompt %s", w.ai.prompt())
	}
}

func TestPromptShowsCollections(t *testing.T) {
	w := newWorld(t, answers("ok"))
	notes := sqlite.Notes{DB: w.db}
	if err := notes.Insert(ctx, w.user.ID, &dom.Note{ID: 1, Collection: "Кулинарная книга", Title: "Шарлотка", Content: "яблоки", CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	w.say(t, dom.RoleUser, "сохрани рецепт", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})

	prompt := w.ai.prompt()
	if !strings.Contains(prompt, "Collections of Alex: Кулинарная книга") {
		t.Fatalf("prompt %s", prompt)
	}
}

func TestMemoriesFillUpFromThePreviousExchange(t *testing.T) {
	w := newWorld(t, answers("ok"))
	m := &dom.Memory{ID: w.ids.Next(), Name: "Печь", Category: "work", Content: "Печь для обжига на 1220", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := w.dasyra.Memories.Insert(ctx, w.user.ID, m); err != nil {
		t.Fatal(err)
	}
	w.say(t, dom.RoleUser, "у меня проблема с печью для обжига", time.Now().Add(-2*time.Minute))
	w.say(t, dom.RoleAssistant, "Поняла, разбирайся", time.Now().Add(-time.Minute))
	w.say(t, dom.RoleUser, "что ещё мне нужно было сделать сегодня?", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})
	if !strings.Contains(w.ai.prompt(), "Печь для обжига на 1220") {
		t.Fatalf("prompt %s", w.ai.prompt())
	}
}

func TestPromptShowsTheToDoList(t *testing.T) {
	w := newWorld(t, answers("ok"))
	todos := sqlite.Todos{DB: w.db}
	yesterday := time.Now().Add(-24 * time.Hour)
	for _, todo := range []*dom.Todo{
		{ID: 1, Text: "Выставить фактуру", Day: time.Now().Format(dom.DayLayout), CreatedAt: time.Now()},
		{ID: 2, Text: "Помыть кисти", CreatedAt: time.Now()},
		{ID: 3, Text: "Старое дело", Day: yesterday.Format(dom.DayLayout), DoneAt: &yesterday, CreatedAt: yesterday},
	} {
		if err := todos.Insert(ctx, w.user.ID, todo); err != nil {
			t.Fatal(err)
		}
	}
	w.say(t, dom.RoleUser, "что осталось?", time.Now())
	w.dasyra.Speak(ctx, w.user, Run{})

	prompt := w.ai.prompt()
	if !strings.Contains(prompt, "To-do list of Alex") || !strings.Contains(prompt, "today: Выставить фактуру") ||
		!strings.Contains(prompt, "#2 no day: Помыть кисти") || strings.Contains(prompt, "Старое дело") {
		t.Fatalf("prompt %s", prompt)
	}
}

func TestEachModeRunsAsItsOwnRole(t *testing.T) {
	for run, role := range map[Run]string{{}: llm.RoleDasyra, {Proactive: true}: llm.RoleCheckIn,
		{Proactive: true, Trigger: "Due event"}: llm.RoleReminder, {Trigger: "follow-up"}: llm.RoleDasyra} {
		if got := run.model().Role; got != role {
			t.Errorf("%+v runs as %s, want %s", run, got, role)
		}
	}
}
