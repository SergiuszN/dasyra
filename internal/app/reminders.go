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

const dueTrigger = `EVENT DUE NOW: %[1]s. If this is a task you agreed to perform for %[2]s (a digest, summary or check), do that task now and send its result. Use its "skill" memory first (list_memories); reuse working sources and APIs with fetch_json. Search only for needed fresh evidence or a specific gap; when a page fails, find another source or public API rather than repeating it. Compare with the last successful check, including its period and latest item. Before replying, update the skill with the checked period, latest result, working source/API + complete jq filter, and useful failure notes. If nothing materially changed, send one short sentence saying so, without old details. Promise only the scheduled delivery: a recurring check is not continuous monitoring or an instant alert. Otherwise remind %[2]s about it now, briefly. Stay on this event, do not append unrelated questions or reminders. Do not reply NO_MESSAGE.`

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
