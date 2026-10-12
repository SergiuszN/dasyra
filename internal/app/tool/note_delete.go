package tool

import (
	"context"
	"fmt"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type DeleteNote struct {
	Notes dom.Notes
}

func (DeleteNote) Spec() llm.Tool {
	return Spec("delete_note", "Delete a saved note by id", Int("id", "Note id").Required())
}

func (t DeleteNote) Run(ctx context.Context, s *Session, a Args) (string, error) {
	id := a.Int("id")
	n, err := t.Notes.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if n == nil {
		return fmt.Sprintf("Error: note #%d not found.", id), nil
	}
	if err := t.Notes.Delete(ctx, s.User.ID, id); err != nil {
		return "", err
	}
	return fmt.Sprintf("Note #%d %q deleted from %q.", n.ID, n.Title, n.Collection), nil
}
