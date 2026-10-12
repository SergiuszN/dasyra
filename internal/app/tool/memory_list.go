package tool

import (
	"context"
	"fmt"
	"strconv"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type ListMemories struct {
	Memories dom.Memories
	Recall   Recall
}

func (ListMemories) Spec() llm.Tool {
	return Spec("list_memories", "List/search memories by text, id, or category",
		Str("query", "Text query or exact id. If no query provided, returns all memories with id, name and category only"),
		Str("category", "Category filter"),
		Int("offset", "Offset (default 0)"),
		Int("limit", "Max items (default 20)"),
	)
}

type memoryRow struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	Content   string `json:"content"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type memoryPage struct {
	Count      int    `json:"count"`
	Offset     int    `json:"offset"`
	Limit      int    `json:"limit"`
	Message    string `json:"message,omitempty"`
	Memories   any    `json:"memories,omitempty"`
	NextOffset *int   `json:"next_offset,omitempty"`
}

func (t ListMemories) Run(ctx context.Context, s *Session, a Args) (string, error) {
	query, offset, limit := a.String("query"), a.IntOr("offset", 0), a.IntOr("limit", 20)
	memories, err := t.find(ctx, s.User.ID, query, a.String("category"), offset, limit)
	if err != nil {
		return "", err
	}
	if len(memories) == 0 {
		return toJSON(memoryPage{Offset: offset, Limit: limit, Message: "No memories found matching the criteria."}), nil
	}

	page := memoryPage{Count: len(memories), Offset: offset, Limit: limit, NextOffset: nextOffset(len(memories), offset, limit)}
	if query == "" {
		// Titles only: update_memory rewrites the whole content and must see it first
		titles := make([]string, len(memories))
		for i, m := range memories {
			titles[i] = fmt.Sprintf("ID: %d | Name: %s | Category: %s", m.ID, m.Name, m.Category)
		}
		page.Memories = titles
		return toJSON(page), nil
	}

	rows := make([]memoryRow, len(memories))
	for i, m := range memories {
		s.Memories.Mark(m.ID)
		rows[i] = memoryRow{m.ID, m.Name, m.Category, m.Content, m.CreatedAt.Format(dom.ShortTime), m.UpdatedAt.Format(dom.ShortTime)}
	}
	page.Memories = rows
	return toJSON(page), nil
}

func (t ListMemories) find(ctx context.Context, userID int64, query, category string, offset, limit int) ([]dom.Memory, error) {
	if id, err := strconv.ParseInt(query, 10, 64); err == nil {
		m, err := t.Memories.Get(ctx, userID, id)
		if m == nil || err != nil {
			return nil, err
		}
		return []dom.Memory{*m}, nil
	}
	if query == "" {
		return t.Memories.List(ctx, userID, category, offset, limit)
	}
	if category == "" && offset == 0 {
		return t.Recall.Find(ctx, userID, []string{query}, limit)
	}
	return t.Memories.Search(ctx, userID, dom.SearchTerms(query), category, offset, limit)
}

// nextOffset points at the next page when this one came full.
func nextOffset(count, offset, limit int) *int {
	if count != limit {
		return nil
	}
	next := offset + limit
	return &next
}
