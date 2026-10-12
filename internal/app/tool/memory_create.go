package tool

import (
	"context"
	"fmt"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// IDs hands out row ids, time ordered: a newer message always has a bigger id.
type IDs interface{ Next() int64 }

type CreateMemory struct {
	Memories dom.Memories
	IDs      IDs
}

func (CreateMemory) Spec() llm.Tool {
	return Spec("create_memory", "Create a new memory for a topic that has no memory yet",
		Str("name", "Memory title").Required(),
		Str("category", `Memory category. "skill" is a how-to for a recurring task: working sources, API URLs with jq filters, and sites that fail`).Required(),
		Str("content", "Memory content").Required(),
	)
}

func (t CreateMemory) Run(ctx context.Context, s *Session, a Args) (string, error) {
	name, category, content := a.String("name"), a.String("category"), a.String("content")
	if name == "" || category == "" || content == "" {
		return "Error: name, category and content are required.", nil
	}

	now := time.Now()
	m := &dom.Memory{ID: t.IDs.Next(), Name: name, Category: category, Content: content, CreatedAt: now, UpdatedAt: now}
	if err := t.Memories.Insert(ctx, s.User.ID, m); err != nil {
		return "", err
	}
	s.Memories.Mark(m.ID)
	return fmt.Sprintf("Memory created with id %d.", m.ID), nil
}
