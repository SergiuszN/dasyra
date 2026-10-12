package tool

import (
	"context"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type ListEvents struct {
	Events dom.Events
}

func (ListEvents) Spec() llm.Tool {
	return Spec("list_events", "List/search events by recurrence, text, and dates",
		Bool("recurring", "true recurring, false one-time, omit all"),
		Str("query", "Text query or exact id"),
		Str("from", "Start datetime (default now unless recurring=true)"),
		Str("to", "End datetime (Y-m-d H:i:s)"),
		Int("offset", "Offset (default 0)"),
		Int("limit", "Max items (default 10)"),
	)
}

type eventRow struct {
	ID                int64   `json:"id"`
	Event             string  `json:"event"`
	StartTime         string  `json:"start_time"`
	EndTime           *string `json:"end_time"`
	Location          *string `json:"location"`
	RepeatInterval    *string `json:"repeat_interval"`
	RecurrenceEndDate *string `json:"recurrence_end_date"`
}

type eventPage struct {
	Count      int        `json:"count"`
	Offset     int        `json:"offset"`
	Limit      int        `json:"limit"`
	Message    string     `json:"message,omitempty"`
	Events     []eventRow `json:"events,omitempty"`
	NextOffset *int       `json:"next_offset,omitempty"`
}

func (t ListEvents) Run(ctx context.Context, s *Session, a Args) (string, error) {
	f := dom.EventFilter{Query: a.String("query"), Offset: a.IntOr("offset", 0), Limit: a.IntOr("limit", 10)}
	if recurring, ok := a.Bool("recurring"); ok {
		f.Recurring = &recurring
	}
	var err error
	if f.From, err = parseOptional(s, a, "from"); err != nil {
		return "", err
	}
	if f.To, err = parseOptional(s, a, "to"); err != nil {
		return "", err
	}
	if f.Recurring == nil && f.From == nil {
		now := s.User.Local(time.Now())
		f.From = &now
	}

	events, err := t.Events.Find(ctx, s.User, f)
	if err != nil {
		return "", err
	}
	if len(events) == 0 {
		return toJSON(eventPage{Offset: f.Offset, Limit: f.Limit, Message: "No events found matching the criteria."}), nil
	}

	rows := make([]eventRow, len(events))
	for i, e := range events {
		s.Events.Mark(e.ID)
		rows[i] = eventRow{
			ID: e.ID, Event: e.Title, StartTime: e.Start.Format(dom.DateTimeLayout),
			EndTime: formatTime(e.End), Location: nonEmpty(e.Location), RepeatInterval: nonEmpty(e.RepeatDays), RecurrenceEndDate: formatTime(e.RecurrenceEnd),
		}
	}
	return toJSON(eventPage{Count: len(rows), Offset: f.Offset, Limit: f.Limit, Events: rows, NextOffset: nextOffset(len(rows), f.Offset, f.Limit)}), nil
}

func parseOptional(s *Session, a Args, key string) (*time.Time, error) {
	if !a.Has(key) {
		return nil, nil
	}
	t, err := s.User.ParseLocal(a.String(key))
	return &t, err
}

func formatTime(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.Format(dom.DateTimeLayout)
	return &s
}

func nonEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
