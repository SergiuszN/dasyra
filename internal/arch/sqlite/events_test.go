package sqlite

import (
	"testing"
	"time"

	"dasyra/internal/dom"
)

var kyivUser = &dom.User{ID: 1, Timezone: "Europe/Kyiv"}

func kyiv(s string) time.Time {
	t, err := time.ParseInLocation(dom.DateTimeLayout, s, kyivUser.Location())
	if err != nil {
		panic(err)
	}
	return t
}

func TestEventsKeepTheUsersWallClock(t *testing.T) {
	db := openTest(t)
	events := Events{db}
	must(t, events.Insert(ctx, kyivUser, &dom.Event{ID: 1, Title: "Call", Start: kyiv("2026-10-10 09:00:00")}))

	var raw string
	must(t, db.QueryRow("SELECT CAST(start_time AS TEXT) FROM calendar_events WHERE id = 1").Scan(&raw))
	got, _ := events.Get(ctx, kyivUser, 1)
	if raw != "2026-10-10 09:00:00" || !got.Start.Equal(kyiv("2026-10-10 09:00:00")) || got.End != nil || got.Recurring() {
		t.Fatalf("raw %q, event %+v", raw, got)
	}
}

func TestUpcomingAndRecurring(t *testing.T) {
	events := Events{openTest(t)}
	now := kyiv("2026-10-09 12:00:00")
	for _, e := range []dom.Event{
		{ID: 1, Title: "yesterday", Start: kyiv("2026-10-08 13:00:00")},
		{ID: 2, Title: "two days ago", Start: kyiv("2026-10-07 13:00:00")},
		{ID: 3, Title: "next week", Start: kyiv("2026-10-17 13:00:00")},
		{ID: 4, Title: "series", Start: kyiv("2026-10-01 08:00:00"), RepeatDays: "mon"},
		{ID: 5, Title: "ended", Start: kyiv("2026-01-01 08:00:00"), RepeatDays: "mon", RecurrenceEnd: ptr(kyiv("2026-03-01 00:00:00"))},
	} {
		must(t, events.Insert(ctx, kyivUser, &e))
	}
	upcoming, _ := events.Upcoming(ctx, kyivUser, now, 6)
	if len(upcoming) != 1 || upcoming[0].ID != 1 {
		t.Fatalf("upcoming %+v", upcoming)
	}
	recurring, _ := events.Recurring(ctx, kyivUser, now)
	if len(recurring) != 1 || recurring[0].ID != 4 {
		t.Fatalf("recurring %+v", recurring)
	}
}

func TestDueOnceAndMovedEventIsDueAgain(t *testing.T) {
	events := Events{openTest(t)}
	from, to := kyiv("2026-10-09 08:30:00"), kyiv("2026-10-09 09:00:00")
	e := &dom.Event{ID: 1, Title: "Call mom", Start: kyiv("2026-10-09 08:55:00")}
	must(t, events.Insert(ctx, kyivUser, e))
	must(t, events.Insert(ctx, kyivUser, &dom.Event{ID: 2, Title: "Digest", Start: kyiv("2026-10-01 08:45:00"), RepeatDays: "fri"}))

	due, _ := events.Due(ctx, kyivUser, from, to)
	if len(due) != 2 || due[0].ID != 1 || due[1].ID != 2 {
		t.Fatalf("due %+v", due)
	}
	must(t, events.MarkReminded(ctx, kyivUser, 1, to))
	must(t, events.MarkReminded(ctx, kyivUser, 2, to))
	if due, _ := events.Due(ctx, kyivUser, from, to); len(due) != 0 {
		t.Fatalf("reminded events are due again: %+v", due)
	}

	e.Start = kyiv("2026-10-09 08:58:00")
	must(t, events.Update(ctx, kyivUser, e))
	if due, _ := events.Due(ctx, kyivUser, from, to); len(due) != 1 {
		t.Fatal("a moved event must be reminded about again")
	}
}

func TestFindEvents(t *testing.T) {
	events := Events{openTest(t)}
	must(t, events.Insert(ctx, kyivUser, &dom.Event{ID: 1, Title: "Gym", Start: kyiv("2026-10-10 18:00:00"), Location: "Fitness Park"}))
	must(t, events.Insert(ctx, kyivUser, &dom.Event{ID: 2, Title: "Digest", Start: kyiv("2026-10-01 08:00:00"), RepeatDays: "mon"}))

	byID, _ := events.Find(ctx, kyivUser, dom.EventFilter{Query: "2", Limit: 10})
	byText, _ := events.Find(ctx, kyivUser, dom.EventFilter{Query: "park", Limit: 10})
	recurring, _ := events.Find(ctx, kyivUser, dom.EventFilter{Recurring: ptr(true), Limit: 10})
	from := kyiv("2026-10-05 00:00:00")
	future, _ := events.Find(ctx, kyivUser, dom.EventFilter{From: &from, Limit: 10})
	if len(byID) != 1 || byID[0].ID != 2 || len(byText) != 1 || len(recurring) != 1 || len(future) != 1 || future[0].ID != 1 {
		t.Fatalf("byID %v byText %v recurring %v future %v", byID, byText, recurring, future)
	}
}

func ptr[T any](v T) *T { return &v }
