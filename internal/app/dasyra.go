package app

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/dom"
)

const (
	maxIterations = 15
	// Messages that keep coming while she answers get follow-up replies, a few at most per lock
	maxFollowUps = 3
	// Luna checked every item of a gift list, 30-40 web calls and 5 minutes per reply. The nudge comes only
	// once she gets there, a budget announced upfront would become a target
	webCallsNudge = 20
	webCallsLimit = 40
	errorText     = "Oops, something broke on my side. Please write to me a bit later."
)

var webTools = []string{tool.WebSearchName, tool.WebFetchName, tool.FetchJSONName}

// Run says why Dasyra speaks: a reply to the user, a check-in, or a trigger such as a due reminder.
type Run struct {
	Proactive   bool
	Trigger     string // why she speaks up, e.g. a due event
	MemoryQuery string // what to look up in memory instead of the latest user message
}

func (r Run) checkIn() bool { return r.Proactive && r.Trigger == "" }

func (r Run) purpose() string {
	switch {
	case !r.Proactive:
		return llm.PurposeChat
	case r.Trigger == "":
		return llm.PurposeCheckIn
	}
	return llm.PurposeReminder
}

// Dasyra writes replies, check-ins and reminders: one endless chat with memory and a calendar.
type Dasyra struct {
	Users     dom.Users
	Messages  dom.Messages
	Memories  dom.Memories
	Events    dom.Events
	Notes     dom.Notes
	Todos     dom.Todos
	Recall    tool.Recall
	ToolCalls dom.ToolCalls
	Tools     *tool.Registry
	AI        llm.Client
	Chat      Messenger
	IDs       tool.IDs
	Log       *slog.Logger
	PartPause time.Duration // between the messages of one reply, like a person typing

	locks userLocks
}

// Speak runs Dasyra once for the user and answers messages that arrived meanwhile.
func (d *Dasyra) Speak(ctx context.Context, u *dom.User, run Run) {
	defer d.locks.lock(u.ID)()
	defer func() {
		// One broken run must not take the whole bot down
		if r := recover(); r != nil {
			d.Log.Error("Dasyra run panicked", "user_id", u.ID, "panic", r)
		}
	}()
	ctx = llm.For(ctx, u.ID, run.purpose())

	// A message that arrives while a reply runs is not in its context, and its own handler finds the reply
	// already stored and skips: a tester's second question got no answer. Whoever holds the lock answers it
	seen, ok := d.speak(ctx, u, run, false)
	for i := 0; ok && i < maxFollowUps && d.hasNewMessage(ctx, u, seen); i++ {
		followUp := Run{Trigger: fmt.Sprintf("Your last reply was written before you saw the latest message(s) from %s: in the history they may come before it. Answer what is still unanswered now, do not repeat that reply.", u.DisplayName())}
		seen, ok = d.speak(llm.For(ctx, 0, llm.PurposeChat), u, followUp, true)
	}
}

func (d *Dasyra) hasNewMessage(ctx context.Context, u *dom.User, seen int64) bool {
	has, err := d.Messages.HasUserMessageAfter(ctx, u.ID, seen)
	return err == nil && has
}

// speak returns the newest message the stored reply has seen, false when nothing was stored.
func (d *Dasyra) speak(ctx context.Context, u *dom.User, run Run, followUp bool) (int64, bool) {
	session := tool.NewSession(u)
	history, err := d.history(ctx, u)
	if err != nil {
		d.Log.Error("Failed to read history", "user_id", u.ID, "error", err)
		return 0, false
	}
	// The newest message is already answered: it came while the previous reply ran and got a follow-up from it
	if newest := history.Newest(); !run.Proactive && !followUp && newest != nil && newest.Role == dom.RoleAssistant {
		return 0, false
	}

	messages := d.prompt(ctx, session, history, run)
	if !run.Proactive {
		defer d.typing(ctx, u)()
	}
	calls := &callLog{}
	answer := dom.Answer(llm.Loop{Client: d.AI, Model: llm.Dasyra, MaxIterations: maxIterations, ErrorText: errorText, Log: d.Log}.
		Run(ctx, messages, d.Tools.Specs(), d.executor(session, calls)))

	if !d.worthSending(u, run, answer) {
		return 0, false
	}
	reply := &dom.Message{ID: d.IDs.Next(), Role: dom.RoleAssistant, Content: string(answer), CreatedAt: now(), CheckIn: run.checkIn()}
	if err := d.Messages.Insert(ctx, u.ID, reply); err != nil {
		d.Log.Error("Failed to store the reply", "user_id", u.ID, "error", err)
		return 0, false
	}
	if len(calls.calls) > 0 {
		if err := d.ToolCalls.SaveRun(ctx, u.ID, reply.ID, calls.calls); err != nil {
			d.Log.Error("Failed to store tool calls", "user_id", u.ID, "error", err)
		}
	}

	var seen int64
	if newest := history.Newest(); newest != nil {
		seen = newest.ID
	}
	d.deliver(ctx, u, history, answer)
	return seen, true
}

func (d *Dasyra) history(ctx context.Context, u *dom.User) (dom.History, error) {
	count, err := d.Messages.Count(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	return d.Messages.Last(ctx, u.ID, dom.HistoryLimit(count))
}

// worthSending drops empty answers, NO_MESSAGE, a failed check-in and a check-in that only reacts.
func (d *Dasyra) worthSending(u *dom.User, run Run, answer dom.Answer) bool {
	_, isReaction := answer.Reaction()
	switch {
	case answer == "":
		d.Log.Error("Failed to get a valid response from AI after maximum iterations.", "user_id", u.ID)
		return false
	case answer.Silent():
		return false
	case run.Proactive && answer == errorText:
		// Nobody asked for a check-in, so a failed one stays in the logs instead of an error in the chat
		d.Log.Error("Proactive run failed, nothing sent.", "user_id", u.ID)
		return false
	case run.Proactive && isReaction:
		return false
	}
	return true
}

// deliver sends the answer as a few messages, or taps a reaction on the user's last message.
// A reaction is stored as a reply anyway: an unanswered message would be answered again as a dropped one.
func (d *Dasyra) deliver(ctx context.Context, u *dom.User, history dom.History, answer dom.Answer) {
	if emoji, ok := answer.Reaction(); ok {
		last := history.LastFromUser()
		if last == nil || last.TelegramID == 0 || d.Chat.React(ctx, u.ChatID, last.TelegramID, emoji) != nil {
			d.Log.Error("Reaction failed, nothing sent.", "user_id", u.ID, "reaction", emoji)
		}
		return
	}
	for i, part := range answer.Parts() {
		if i > 0 {
			d.Chat.Typing(ctx, u.ChatID)
			time.Sleep(d.PartPause)
		}
		if err := d.Chat.Send(ctx, u.ChatID, part); err != nil {
			d.Log.Error("Failed to send a reply part", "user_id", u.ID, "error", err)
		}
	}
}

// typing keeps "typing…" on while she thinks, also through long tool calls; the returned func stops it.
func (d *Dasyra) typing(ctx context.Context, u *dom.User) func() {
	ctx, stop := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(4 * time.Second)
		defer ticker.Stop()
		for {
			d.Chat.Typing(ctx, u.ChatID)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
	return stop
}

// callLog collects the tool calls of one reply and counts its web calls.
type callLog struct {
	calls []dom.ToolCall
	web   int
}

func (d *Dasyra) executor(s *tool.Session, log *callLog) llm.Executor {
	return func(ctx context.Context, name, arguments string) string {
		isWeb := slices.Contains(webTools, name)
		if isWeb && log.web >= webCallsLimit {
			return "Not executed: the web limit for this reply is reached. Answer with what you have and briefly say what you could not check."
		}

		result, args := d.Tools.Execute(ctx, s, name, arguments)
		if isWeb {
			if log.web++; log.web >= webCallsNudge {
				result += fmt.Sprintf("\n\n[%d web calls in this reply. If you already have enough for a good answer, stop and answer.]", log.web)
			}
		}
		if name != tool.RecentResultsName {
			log.calls = append(log.calls, dom.ToolCall{Tool: name, Args: args.JSON(), Result: result})
		}
		return result
	}
}
