package dom

import (
	"testing"
	"time"
)

func TestCheckInPlanThinsOutForSilentUsers(t *testing.T) {
	p := DefaultCheckInPlan()
	day := 24 * time.Hour
	cases := []struct {
		hour   int
		silent time.Duration
		want   bool
	}{
		{9, time.Hour, true},
		{9, 2*day - time.Minute, true},
		{9, 2 * day, false},
		{15, 2 * day, true}, // the middle of 9, 12, 15, 18, 21
		{15, 5 * day, false},
	}
	for _, c := range cases {
		if got := p.Due(c.hour, c.silent); got != c.want {
			t.Errorf("Due(%d, %v) = %v", c.hour, c.silent, got)
		}
	}

	p.QuietDays, p.StopDays = 0, 0
	if !p.Due(9, 100*day) {
		t.Fatal("rules off")
	}
	if (CheckInPlan{}).QuietHour() != -1 {
		t.Fatal("no hours")
	}
}

func TestCheckInPlanProblem(t *testing.T) {
	p := CheckInPlan{Hours: []int{18, 9, 18}, QuietDays: 2, StopDays: 5, Threshold: 0.3}
	if p.Problem() != "" || len(p.Hours) != 2 || p.Hours[0] != 9 {
		t.Fatalf("hours %v", p.Hours)
	}
	for _, bad := range []CheckInPlan{{Hours: []int{24}}, {QuietDays: -1}, {Threshold: 1.5}} {
		if bad.Problem() == "" {
			t.Errorf("accepted %+v", bad)
		}
	}
}
