package tool

import (
	"context"
	"fmt"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// SendNote delivers a saved note to the chat as it is: the model never retypes a recipe, so amounts stay right
// and long texts cost no output tokens. The chat keeps only a short line about it, not the whole text.
type SendNote struct {
	Notes    dom.Notes
	Messages dom.Messages
	Chat     Sender
	IDs      IDs
}

func (SendNote) Spec() llm.Tool {
	return Spec("send_note", "Send a saved note to the chat exactly as saved, instead of retyping it. Your reply comes after it, so keep it to a short line or two.",
		Int("id", "Note id").Required(),
	)
}

func (t SendNote) Run(ctx context.Context, s *Session, a Args) (string, error) {
	id := a.Int("id")
	n, err := t.Notes.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if n == nil {
		return fmt.Sprintf("Error: note #%d not found.", id), nil
	}
	if err := t.Chat.Send(ctx, s.User.ChatID, n.Text()); err != nil {
		return fmt.Sprintf("Error: Telegram did not take the note (%s), nothing was sent.", err), nil
	}

	sent := &dom.Message{ID: t.IDs.Next(), Role: dom.RoleAssistant, Content: fmt.Sprintf("[sent note #%d %q from %q]", n.ID, n.Title, n.Collection), CreatedAt: time.Now()}
	if err := t.Messages.Insert(ctx, s.User.ID, sent); err != nil {
		return "", err
	}
	return fmt.Sprintf("Note #%d %q was sent to the chat.", n.ID, n.Title), nil
}
