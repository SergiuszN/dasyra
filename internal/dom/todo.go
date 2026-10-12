package dom

import (
	"fmt"
	"time"
)

const DayLayout = "2006-01-02"

// Todo is one item of the user's to-do list, planned for a day of the user's calendar or for no day.
type Todo struct {
	ID        int64
	Text      string
	Day       string // DayLayout in the user's zone, "" for no day
	DoneAt    *time.Time
	CreatedAt time.Time
}

func (t *Todo) Done() bool { return t.DoneAt != nil }

// Line is how the model sees the item: "#3 2026-10-12 Mon, overdue: call the bank" or "... done at 10:49".
func (t *Todo) Line(u *User, today string) string {
	when := "no day"
	if day, err := time.Parse(DayLayout, t.Day); err == nil {
		when = day.Format("2006-01-02 Mon")
		switch {
		case t.Day == today:
			when += ", today"
		case t.Day < today && !t.Done():
			when += ", overdue"
		}
	}
	line := fmt.Sprintf("#%d %s: %s", t.ID, when, t.Text)
	if t.Done() {
		line += ", done at " + u.Local(*t.DoneAt).Format("2006-01-02 15:04")
	}
	return line
}

// ParseDay reads a day the model gives as a date or a date with a time, "" stays no day.
func (u *User) ParseDay(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := u.ParseLocal(s)
	if err != nil {
		return "", err
	}
	return t.Format(DayLayout), nil
}
