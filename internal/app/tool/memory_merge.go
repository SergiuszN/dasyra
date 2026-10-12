package tool

import (
	"context"
	"fmt"
	"slices"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// MergeMemories keeps the first memory with the merged content and deletes the others.
type MergeMemories struct {
	Memories dom.Memories
}

func (MergeMemories) Spec() llm.Tool {
	return Spec("merge_memories", "Merge two or more memories about the same topic into one: the first id is kept with the new name and content, the others are deleted",
		List("ids", "integer", "Ids of memories to merge, at least two").Required(),
		Str("name", "Title of the merged memory").Required(),
		Str("category", "Category of the merged memory").Required(),
		Str("content", "Merged content with every fact from all merged memories").Required(),
	)
}

func (t MergeMemories) Run(ctx context.Context, s *Session, a Args) (string, error) {
	ids := unique(a.Ints("ids"))
	name, category, content := a.String("name"), a.String("category"), a.String("content")
	if len(ids) < 2 {
		return "Error: merge needs at least two memory ids.", nil
	}
	if name == "" || category == "" || content == "" {
		return "Error: name, category and content are required.", nil
	}

	memories := make([]*dom.Memory, 0, len(ids))
	for _, id := range ids {
		m, err := t.Memories.Get(ctx, s.User.ID, id)
		if err != nil {
			return "", err
		}
		if m == nil {
			return fmt.Sprintf("Error: memory with id %d not found, nothing merged.", id), nil
		}
		memories = append(memories, m)
	}

	kept := memories[0]
	kept.Name, kept.Category, kept.Content, kept.UpdatedAt = name, category, content, time.Now()
	if err := t.Memories.Update(ctx, s.User.ID, kept); err != nil {
		return "", err
	}
	for _, m := range memories[1:] {
		if err := t.Memories.Delete(ctx, s.User.ID, m.ID); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("Merged %d memories into #%d.", len(ids), kept.ID), nil
}

func unique(ids []int64) []int64 {
	var out []int64
	for _, id := range ids {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}
