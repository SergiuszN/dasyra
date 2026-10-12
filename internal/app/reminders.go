package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"dasyra/internal/dom"
)

const (
	// Catches events missed by a skipped run, older ones are not reminded about
	reminderLookback = 30 * time.Minute
	// A live reply takes up to a few minutes, a message waiting longer than that was dropped
	forgottenAfter = 6 * time.Minute
	forgottenUntil = 30 * time.Minute
)

const dueTrigger = `EVENT DUE NOW: %[1]s. If it is something you agreed to do for %[2]s (a digest, a summary, a check), do it now (search the web if it needs fresh data) and send the result. For such a task start from its "skill" memory if there is one (list_memories), and before answering save or update it with what worked (sources, API URLs with jq filters) and what failed, so the next run does not repeat dead ends. When pages give no data (blocked, empty, rendered by JavaScript), search for an open API of that data ("<topic or site> API json") and try it with fetch_json. If there is nothing new since the last run, say so in one short message instead of repeating it. Otherwise remind %[2]s about it right now, briefly and naturally, with any useful context. Do not reply NO_MESSAGE.`

// Reminders wakes Dasyra for due events and occurrences of recurring ones (digests, regular tasks),
// and answers messages whose reply was killed midway.
type Reminders struct {
	Users    dom.Users
	Messages dom.Messages
	Events   dom.Events
	Dasyra   *Dasyra
	Log      *slog.Logger
}

func (r *Reminders) Run(ctx context.Context) error {
	users, err := r.Users.AllWithChat(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		r.remind(ctx, u)
		r.answerForgotten(ctx, u)
	}
	return nil
}

func (r *Reminders) remind(ctx context.Context, u *dom.User) {
	// Events are stored in the user's wall-clock time, so the window is too
	to := u.Local(now())
	due, err := r.Events.Due(ctx, u, to.Add(-reminderLookback), to)
	if err != nil {
		r.Log.Error("Reminders failed", "user_id", u.ID, "error", err)
		return
	}
	for _, e := range due {
		// Marked first, so a failing run never repeats the same reminder every few minutes
		if err := r.Events.MarkReminded(ctx, u, e.ID, to); err != nil {
			r.Log.Error("Failed to mark a reminder", "event_id", e.ID, "error", err)
			continue
		}
		r.Dasyra.Speak(ctx, u, Run{Proactive: true, Trigger: fmt.Sprintf(dueTrigger, e.String(), u.DisplayName()), MemoryQuery: e.Title})
	}
}

// answerForgotten picks up a reply killed midway (a restart, a crash) instead of leaving the user waiting.
func (r *Reminders) answerForgotten(ctx context.Context, u *dom.User) {
	last, err := r.Messages.Last(ctx, u.ID, 1)
	newest := last.Newest()
	if err != nil || newest == nil || newest.Role != dom.RoleUser {
		return
	}
	if age := now().Sub(newest.CreatedAt); age < forgottenAfter || age > forgottenUntil {
		return
	}
	r.Log.Info("Answering a forgotten message.", "user_id", u.ID)
	r.Dasyra.Speak(ctx, u, Run{})
}
