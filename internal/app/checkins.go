package app

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
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
	Plans    dom.CheckInPlans
	Dasyra   *Dasyra
	Gate     llm.Decider    // asks whether a check-in is worth running when the plan turns it on
	Zone     *time.Location // of the plan's hours, the server's; nil is the process zone
	Log      *slog.Logger
}

// Scheduled runs every full hour and checks in at the hours of the plan.
func (c *CheckIns) Scheduled(ctx context.Context) error {
	plan := c.plan(ctx)
	if !slices.Contains(plan.Hours, c.hour()) {
		return nil
	}
	return c.run(ctx, plan)
}

// Run checks in now, whatever the hour: dasyra proactive.
func (c *CheckIns) Run(ctx context.Context) error {
	return c.run(ctx, c.plan(ctx))
}

func (c *CheckIns) run(ctx context.Context, plan dom.CheckInPlan) error {
	c.Log.Info("Proactive check-in triggered")
	users, err := c.Users.AllWithChat(ctx)
	if err != nil {
		return err
	}
	for _, u := range users {
		if c.due(ctx, u, plan) && c.worthIt(ctx, u, plan) {
			c.Dasyra.Speak(ctx, u, Run{Proactive: true})
		}
	}
	return nil
}

func (c *CheckIns) plan(ctx context.Context) dom.CheckInPlan {
	if c.Plans == nil {
		return dom.DefaultCheckInPlan()
	}
	plan, err := c.Plans.CheckInPlan(ctx)
	if err != nil {
		c.Log.Error("Failed to read the check-in plan, using the default", "error", err)
		return dom.DefaultCheckInPlan()
	}
	return plan
}

func (c *CheckIns) hour() int {
	if c.Zone == nil {
		return now().Hour()
	}
	return now().In(c.Zone).Hour()
}

func (c *CheckIns) due(ctx context.Context, u *dom.User, plan dom.CheckInPlan) bool {
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
	// Empty runs don't count as unanswered check-ins: a tester who stopped writing got five runs a day for nothing
	wrote, err := c.Messages.LastUserMessageAt(ctx, u.ID)
	if err != nil || wrote.IsZero() || !plan.Due(c.hour(), now().Sub(wrote)) {
		return false
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

// worthIt asks a decision model whether Dasyra has anything to say, when the plan turns the gate on: 87% of
// check-ins ended in NO_MESSAGE, each a full run of Dasyra. The gate only saves money, so when it fails
// the check-in runs as before.
func (c *CheckIns) worthIt(ctx context.Context, u *dom.User, plan dom.CheckInPlan) bool {
	if !plan.Gate || c.Gate == nil || !u.HasProfile() {
		return true
	}
	ctx = llm.For(ctx, u.ID, llm.PurposeCheckInGate)
	state, err := c.Dasyra.checkInState(ctx, u)
	if err != nil {
		c.Log.Error("Check-in gate failed, checking in anyway", "user_id", u.ID, "error", err)
		return true
	}
	p, err := c.Gate.Probability(ctx, llm.CheckInGate, state, promptText("checkin_gate", "{name}", u.DisplayName()))
	if err != nil {
		c.Log.Error("Check-in gate failed, checking in anyway", "user_id", u.ID, "error", err)
		return true
	}
	c.Log.Info("Check-in gate", "user_id", u.ID, "probability", p, "threshold", plan.Threshold, "pass", p >= plan.Threshold)
	return p >= plan.Threshold
}

// checkInState is what a check-in would see, for the gate: the profile and the context after the system prompt,
// without Dasyra's persona and rules.
func (d *Dasyra) checkInState(ctx context.Context, u *dom.User) ([]string, error) {
	history, err := d.history(ctx, u)
	if err != nil {
		return nil, err
	}
	state := []string{"Profile of " + u.DisplayName() + ":\n" + strings.TrimSpace(u.Profile)}
	for _, m := range d.prompt(ctx, tool.NewSession(u), history, Run{Proactive: true})[1:] {
		state = append(state, m.Text)
	}
	return state, nil
}
