package tool

import (
	"context"
	"fmt"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// UpdateMemory rewrites the content while name and category stay, so a memory can't be turned into another topic.
type UpdateMemory struct {
	Memories dom.Memories
}

func (UpdateMemory) Spec() llm.Tool {
	return Spec("update_memory", "Replace the content of an existing memory on the same topic. Name and category stay as they are.",
		Int("id", "Memory id").Required(),
		Str("content", "Complete new content, keeping every still valid fact").Required(),
	)
}

func (t UpdateMemory) Run(ctx context.Context, s *Session, a Args) (string, error) {
	id, content := a.Int("id"), a.String("content")
	if id == 0 || content == "" {
		return "Error: id and content are required.", nil
	}
	if !s.Memories.Has(id) {
		return fmt.Sprintf("Error: memory #%d was not read in this session, find it with list_memories first to see its content.", id), nil
	}

	m, err := t.Memories.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if m == nil {
		return fmt.Sprintf("Error: memory with id %d not found.", id), nil
	}

	m.Content, m.UpdatedAt = content, time.Now()
	if err := t.Memories.Update(ctx, s.User.ID, m); err != nil {
		return "", err
	}
	return fmt.Sprintf("Memory #%d updated.", id), nil
}
