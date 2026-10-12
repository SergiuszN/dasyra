package app

import (
	"context"
	"log/slog"
	"time"

	"dasyra/internal/dom"
)

const (
	onboardingNudgeAfter = 20 * time.Hour
	// Two check-ins in a row without an answer and they stop until the user writes again: testers got
	// a check-in five times a day for days, each apologizing for the previous ones. Reminders still go out
	maxUnansweredCheckIns = 2
	// A check-in right after a conversation only chases it: testers got one 40 minutes after an unanswered question
	quietAfterTalk = 3 * time.Hour
)

// CheckIns lets Dasyra write first a few times a day, when she has something worth saying.
type CheckIns struct {
	Users    dom.Users
	Messages dom.Messages
	Dasyra   *Dasyra
	Log      *slog.Logger
}

func (c *CheckIns) Run(ctx context.Context) error {
	c.Log.Info("Proactive check-in triggered")
	users, err := c.Users.AllWithChat(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if c.due(ctx, u) {
			c.Dasyra.Speak(ctx, u, Run{Proactive: true})
		}
	}
	return nil
}

func (c *CheckIns) due(ctx context.Context, u *dom.User) bool {
	last, err := c.Messages.Last(ctx, u.ID, 2)
	if err != nil {
		c.Log.Error("Check-in skipped", "user_id", u.ID, "error", err)
		return false
	}
	if newest := last.Newest(); newest != nil && newest.CreatedAt.After(now().Add(-quietAfterTalk)) {
		return false
	}
	if !u.HasProfile() {
		return needsNudge(last)
	}
	// Only check-ins count: with any assistant row counted, a reply plus one check-in already silenced a user
	// for good, and daily digests kept a tester out of check-ins altogether
	unanswered, err := c.Messages.CheckInsSinceUserMessage(ctx, u.ID)
	return err == nil && unanswered < maxUnansweredCheckIns
}

// needsNudge: onboarding happens in live conversation, a user who went quiet gets exactly one nudge:
// the last message is Dasyra's, old enough, and the only one left unanswered.
func needsNudge(last dom.History) bool {
	newest := last.Newest()
	if newest == nil || newest.Role != dom.RoleAssistant || !newest.CreatedAt.Before(now().Add(-onboardingNudgeAfter)) {
		return false
	}
	return len(last) < 2 || last[1].Role != dom.RoleAssistant
}
