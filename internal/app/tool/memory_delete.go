package tool

import (
	"context"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type DeleteMemory struct {
	Memories dom.Memories
}

func (DeleteMemory) Spec() llm.Tool {
	return Spec("delete_memory", "Delete memory by id", Int("id", "Memory id").Required())
}

func (t DeleteMemory) Run(ctx context.Context, s *Session, a Args) (string, error) {
	id := a.Int("id")
	if id == 0 {
		return "Error: memory id is required for deletion.", nil
	}
	if err := t.Memories.Delete(ctx, s.User.ID, id); err != nil {
		return "", err
	}
	return "Memory deleted successfully.", nil
}
