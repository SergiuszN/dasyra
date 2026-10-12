package sqlite

import (
	"fmt"
	"testing"

	"dasyra/internal/dom"
)

func TestCheckInPlanDefaultsAndRoundTrip(t *testing.T) {
	s := Settings{openTest(t)}
	plan, err := s.CheckInPlan(ctx)
	if err != nil || fmt.Sprint(plan) != fmt.Sprint(dom.DefaultCheckInPlan()) {
		t.Fatalf("default %+v %v", plan, err)
	}
	must(t, s.SaveCheckInPlan(ctx, dom.CheckInPlan{Hours: []int{10, 19}, StopDays: 7, Gate: true, Threshold: 0.25}))
	must(t, s.SaveCheckInPlan(ctx, dom.CheckInPlan{Hours: []int{}, StopDays: 7, Gate: true, Threshold: 0.25}))
	plan, _ = s.CheckInPlan(ctx)
	if len(plan.Hours) != 0 || plan.QuietDays != 0 || plan.StopDays != 7 || !plan.Gate || plan.Threshold != 0.25 {
		t.Fatalf("saved %+v", plan)
	}
}
