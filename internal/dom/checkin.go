package dom

import (
	"fmt"
	"slices"
	"time"
)

// CheckInPlan is when Dasyra writes first, set in the admin panel. Without a saved plan DefaultCheckInPlan applies.
type CheckInPlan struct {
	Hours []int // server-time hours, ascending; none turns check-ins off
	// A user silent for QuietDays gets only the middle check-in of the day, one silent for StopDays none until
	// they write again: most check-ins to a silent user were empty runs. 0 turns a rule off
	QuietDays, StopDays int
	// Gate asks a cheap decision model first whether there is anything to say, a check-in runs from Threshold up
	Gate      bool
	Threshold float64
}

func DefaultCheckInPlan() CheckInPlan {
	return CheckInPlan{Hours: []int{9, 12, 15, 18, 21}, QuietDays: 2, StopDays: 5, Threshold: 0.3}
}

// QuietHour is the one check-in a quiet user still gets, the middle hour of the day; -1 without hours.
func (p CheckInPlan) QuietHour() int {
	if len(p.Hours) == 0 {
		return -1
	}
	return p.Hours[len(p.Hours)/2]
}

// Due tells whether a user silent for this long gets the check-in at hour. The hour itself is not checked:
// dasyra proactive checks in at any time.
func (p CheckInPlan) Due(hour int, silent time.Duration) bool {
	switch {
	case p.StopDays > 0 && silent >= days(p.StopDays):
		return false
	case p.QuietDays > 0 && silent >= days(p.QuietDays):
		return hour == p.QuietHour()
	}
	return true
}

// Problem says what is wrong with the plan, "" when it can be saved. Hours get sorted.
func (p *CheckInPlan) Problem() string {
	slices.Sort(p.Hours)
	p.Hours = slices.Compact(p.Hours)
	for _, h := range p.Hours {
		if h < 0 || h > 23 {
			return fmt.Sprintf("hour %d is not between 0 and 23.", h)
		}
	}
	switch {
	case p.QuietDays < 0 || p.StopDays < 0 || p.QuietDays > 365 || p.StopDays > 365:
		return "days must be between 0 and 365."
	case p.Threshold < 0 || p.Threshold > 1:
		return "the gate threshold is a probability between 0 and 1."
	}
	return ""
}

func days(n int) time.Duration { return time.Duration(n) * 24 * time.Hour }
