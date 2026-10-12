package tool

import (
	"context"
	"fmt"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type SaveEvent struct {
	Events dom.Events
	IDs    IDs
}

func (SaveEvent) Spec() llm.Tool {
	return Spec("save_event", "Create or update an event (use id to update)",
		Int("id", "Event id for update"),
		Str("event", "Event title").Required(),
		Str("start_time", "Start datetime (Y-m-d H:i:s)").Required(),
		Str("end_time", "End datetime (Y-m-d H:i:s)"),
		Str("location", "Location"),
		List("repeat_days", "string", "Weekdays a recurring event repeats on at the start_time hour, all seven for daily. Omit for one-time events", dom.Weekdays...),
		Str("recurrence_end_date", "Recurrence end datetime (Y-m-d H:i:s)"),
	)
}

func (t SaveEvent) Run(ctx context.Context, s *Session, a Args) (string, error) {
	title, startRaw := a.String("event"), a.String("start_time")
	if title == "" || startRaw == "" {
		return "Error: event name and start_time are required for saving a calendar event.", nil
	}

	// Stored as the user's wall-clock time, a time sent with an explicit offset is converted to it
	start, err := s.User.ParseLocal(startRaw)
	if err != nil {
		return "Error saving event: " + err.Error(), nil
	}
	end, err := parseOptional(s, a, "end_time")
	if err != nil {
		return "Error saving event: " + err.Error(), nil
	}
	until, err := parseOptional(s, a, "recurrence_end_date")
	if err != nil {
		return "Error saving event: " + err.Error(), nil
	}
	draft := dom.Event{Title: title, Start: start, End: end, Location: a.String("location"),
		RepeatDays: dom.NormalizeRepeatDays(a.Strings("repeat_days")), RecurrenceEnd: until}

	// id 0 or negative means a new event, models often send a placeholder id
	if id := a.Int("id"); id > 0 {
		return t.update(ctx, s, id, draft, a.Has("location"))
	}

	draft.ID = t.IDs.Next()
	if err := t.Events.Insert(ctx, s.User, &draft); err != nil {
		return "", err
	}
	return fmt.Sprintf("Event created successfully with ID: %d", draft.ID), nil
}

// update changes the title and start, and the other fields only when they were passed.
func (t SaveEvent) update(ctx context.Context, s *Session, id int64, d dom.Event, hasLocation bool) (string, error) {
	if !s.Events.Has(id) {
		return "Error: you can only update events that have been read in this session (first list for the event).", nil
	}
	e, err := t.Events.Get(ctx, s.User, id)
	if err != nil {
		return "", err
	}
	if e == nil {
		return fmt.Sprintf("Error: event with ID %d not found.", id), nil
	}

	e.Title, e.Start = d.Title, d.Start
	if d.End != nil {
		e.End = d.End
	}
	if hasLocation {
		e.Location = d.Location
	}
	if d.RepeatDays != "" {
		e.RepeatDays = d.RepeatDays
	}
	if d.RecurrenceEnd != nil {
		e.RecurrenceEnd = d.RecurrenceEnd
	}
	if err := t.Events.Update(ctx, s.User, e); err != nil {
		return "", err
	}
	return "Event updated successfully: " + d.Title, nil
}
