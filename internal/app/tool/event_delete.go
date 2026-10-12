package tool

import (
	"context"
	"fmt"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type DeleteEvent struct {
	Events dom.Events
}

func (DeleteEvent) Spec() llm.Tool {
	return Spec("delete_event", "Delete event by id", Int("id", "Event id").Required())
}

func (t DeleteEvent) Run(ctx context.Context, s *Session, a Args) (string, error) {
	id := a.Int("id")
	if id == 0 {
		return "Error: event id is required for deletion.", nil
	}
	if !s.Events.Has(id) {
		return "Error: you can only delete events that have been read in this session (first search or list for the event).", nil
	}

	e, err := t.Events.Get(ctx, s.User, id)
	if err != nil {
		return "", err
	}
	if e == nil {
		return fmt.Sprintf("Error: event with ID %d not found.", id), nil
	}
	if err := t.Events.Delete(ctx, s.User, id); err != nil {
		return "", err
	}
	return "Event deleted successfully.", nil
}
