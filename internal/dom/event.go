package dom

import (
	"fmt"
	"slices"
	"strings"
	"time"
)

// Weekdays in week order, the form repeat days are stored in ("mon,wed").
var Weekdays = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

const DateTimeLayout = "2006-01-02 15:04:05"

// ShortTime is how the model sees times: the full year and the weekday, "26-10-12" read as the 26th of October.
const ShortTime = "2006-01-02 Mon 15:04"

// Event is a calendar entry in the user's wall-clock time. A recurring one (RepeatDays set) is how
// reminders, digests and other regular tasks wake Dasyra up.
type Event struct {
	ID            int64
	Title         string
	Start         time.Time
	End           *time.Time
	Location      string
	RepeatDays    string // "mon,wed", empty for one-time events
	RecurrenceEnd *time.Time
	RemindedAt    *time.Time
}

func (e *Event) Recurring() bool { return e.RepeatDays != "" }

// Due tells whether the event, or the latest occurrence of a series, fell into (from, to] and was not reminded about yet.
func (e *Event) Due(from, to time.Time) bool {
	if !e.Recurring() {
		return e.RemindedAt == nil && e.Start.After(from) && !e.Start.After(to)
	}
	occurrence, ok := e.LastOccurrence(to)
	if !ok || !occurrence.After(from) {
		return false
	}
	return e.RemindedAt == nil || e.RemindedAt.Before(occurrence)
}

// NormalizeRepeatDays keeps known weekdays in week order ("Wednesday", "mon" -> "mon,wed"), "" when none is left.
func NormalizeRepeatDays(days []string) string {
	wanted := map[string]bool{}
	for _, d := range days {
		d = strings.ToLower(strings.TrimSpace(d))
		if len(d) > 3 {
			d = d[:3]
		}
		wanted[d] = true
	}
	var kept []string
	for _, d := range Weekdays {
		if wanted[d] {
			kept = append(kept, d)
		}
	}
	return strings.Join(kept, ",")
}

// LastOccurrence is the latest occurrence of a recurring event at or before at, looking back no further than yesterday.
func (e *Event) LastOccurrence(at time.Time) (time.Time, bool) {
	at = at.In(e.Start.Location())
	days := strings.Split(e.RepeatDays, ",")
	for back := 0; back <= 1; back++ {
		day := at.AddDate(0, 0, -back)
		occurrence := time.Date(day.Year(), day.Month(), day.Day(), e.Start.Hour(), e.Start.Minute(), 0, 0, at.Location())
		if occurrence.After(at) || occurrence.Before(e.Start) {
			continue
		}
		if e.RecurrenceEnd != nil && occurrence.After(*e.RecurrenceEnd) {
			continue
		}
		if slices.Contains(days, weekdayName(occurrence)) {
			return occurrence, true
		}
	}
	return time.Time{}, false
}

func weekdayName(t time.Time) string {
	return Weekdays[(int(t.Weekday())+6)%7]
}

// String is how an event is shown to the model, with its id so it can be changed directly.
func (e *Event) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "#%d %s on %s", e.ID, e.Title, e.Start.Format(ShortTime))
	if e.End != nil {
		b.WriteString(" to ")
		b.WriteString(e.End.Format(ShortTime))
	}
	if e.Location != "" {
		b.WriteString(" at ")
		b.WriteString(e.Location)
	}
	if e.Recurring() {
		fmt.Fprintf(&b, ", repeats every %s at %s", strings.ReplaceAll(e.RepeatDays, ",", ", "), e.Start.Format("15:04"))
	}
	if e.RecurrenceEnd != nil {
		b.WriteString(" until ")
		b.WriteString(e.RecurrenceEnd.Format("2006-01-02"))
	}
	return b.String()
}
