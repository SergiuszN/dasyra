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

const dueTrigger = `EVENT DUE NOW: %[1]s. If this is work you agreed to perform for %[2]s, do it now without a new message. Start from its "skill" memory; reuse working sources/APIs and find another route for failed pages. Compare with the last successful check. Save the checked period, latest result, source/API + full jq filter and useful failure notes in the skill. If nothing changed, send one short sentence, not old details or promises of instant alerts. Otherwise remind %[2]s about it briefly. Stay on this event. Do not reply NO_MESSAGE.`

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
