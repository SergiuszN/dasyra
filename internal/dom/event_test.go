package dom

import (
	"testing"
	"time"
)

var warsaw, _ = time.LoadLocation("Europe/Warsaw")

func at(s string) time.Time {
	t, err := time.ParseInLocation(DateTimeLayout, s, warsaw)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr[T any](v T) *T { return &v }

func TestNormalizeRepeatDays(t *testing.T) {
	cases := map[string][]string{
		"mon,wed":                     {"Wednesday", " mon "},
		"mon,tue,wed,thu,fri,sat,sun": {"sun", "sat", "fri", "thu", "wed", "tue", "mon"},
		"":                            {"someday", ""},
		"fri":                         {"FRI", "fri"},
	}
	for want, days := range cases {
		if got := NormalizeRepeatDays(days); got != want {
			t.Errorf("NormalizeRepeatDays(%v) = %q, want %q", days, got, want)
		}
	}
}

func TestLastOccurrenceToday(t *testing.T) {
	// 2026-10-09 is a Friday
	e := Event{Start: at("2026-10-01 08:00:00"), RepeatDays: "fri"}
	got, ok := e.LastOccurrence(at("2026-10-09 08:03:00"))
	if !ok || !got.Equal(at("2026-10-09 08:00:00")) {
		t.Fatalf("got %v %v", got, ok)
	}
}

func TestLastOccurrenceYesterday(t *testing.T) {
	e := Event{Start: at("2026-10-01 23:50:00"), RepeatDays: "thu"}
	got, ok := e.LastOccurrence(at("2026-10-09 00:10:00"))
	if !ok || !got.Equal(at("2026-10-08 23:50:00")) {
		t.Fatalf("got %v %v", got, ok)
	}
}

func TestLastOccurrenceSkipsOtherDaysBeforeStartAndAfterEnd(t *testing.T) {
	now := at("2026-10-09 09:00:00")
	cases := map[string]Event{
		"other weekday":    {Start: at("2026-10-01 08:00:00"), RepeatDays: "mon"},
		"series in future": {Start: at("2026-10-09 08:30:00").AddDate(0, 0, 7), RepeatDays: "fri"},
		"series ended":     {Start: at("2026-10-01 08:00:00"), RepeatDays: "fri", RecurrenceEnd: ptr(at("2026-10-08 00:00:00"))},
	}
	for name, e := range cases {
		if _, ok := e.LastOccurrence(now); ok {
			t.Errorf("%s: expected no occurrence", name)
		}
	}
}

func TestDueOneTime(t *testing.T) {
	from, to := at("2026-10-09 08:30:00"), at("2026-10-09 09:00:00")
	e := Event{Start: at("2026-10-09 08:45:00")}
	if !e.Due(from, to) {
		t.Fatal("event inside the window must be due")
	}
	e.RemindedAt = ptr(to)
	if e.Due(from, to) {
		t.Fatal("reminded event must not be due again")
	}
	if (&Event{Start: at("2026-10-09 08:30:00")}).Due(from, to) {
		t.Fatal("the window start is exclusive")
	}
}

func TestDueRecurringOncePerOccurrence(t *testing.T) {
	from, to := at("2026-10-09 07:40:00"), at("2026-10-09 08:10:00")
	e := Event{Start: at("2026-10-01 08:00:00"), RepeatDays: "mon,fri", RemindedAt: ptr(at("2026-10-06 08:01:00"))}
	if !e.Due(from, to) {
		t.Fatal("today's occurrence must be due")
	}
	e.RemindedAt = ptr(at("2026-10-09 08:02:00"))
	if e.Due(from, to) {
		t.Fatal("occurrence already reminded about")
	}
}

func TestEventString(t *testing.T) {
	e := Event{ID: 7, Title: "Gym", Start: at("2026-10-01 18:00:00"), End: ptr(at("2026-10-01 19:30:00")), Location: "Fitness",
		RepeatDays: "mon,wed", RecurrenceEnd: ptr(at("2026-12-31 00:00:00"))}
	want := "#7 Gym on 2026-10-01 Thu 18:00 to 2026-10-01 Thu 19:30 at Fitness, repeats every mon, wed at 18:00 until 2026-12-31"
	if got := e.String(); got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
	if got := (&Event{ID: 1, Title: "Call", Start: at("2026-10-02 09:00:00")}).String(); got != "#1 Call on 2026-10-02 Fri 09:00" {
		t.Fatalf("got %q", got)
	}
}
