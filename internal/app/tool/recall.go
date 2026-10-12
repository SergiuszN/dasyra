package tool

import (
	"context"
	"slices"
	"strings"

	"dasyra/internal/dom"
)

// Recall finds memories by words (FTS) and by meaning, so a message finds a memory written in other words or in
// another language. Without vectors, or when embedding fails, it is FTS alone.
type Recall struct {
	Memories dom.Memories
	Meaning  Meaning
}

// maxByMeaning memories join each text's word hits at most: past three, meaning hits are mostly noise
const maxByMeaning = 3

// Find returns up to limit memories for texts in priority order: the first text fills the list, the next top it up.
func (r Recall) Find(ctx context.Context, userID int64, texts []string, limit int) ([]dom.Memory, error) {
	texts = slices.DeleteFunc(slices.Clone(texts), func(t string) bool { return strings.TrimSpace(t) == "" })
	closest := r.Meaning.Closest(ctx, userID, dom.KindMemory, texts, maxByMeaning)

	var found []dom.Memory
	seen := map[int64]bool{}
	for i, text := range texts {
		if len(found) >= limit {
			break
		}
		var byMeaning []int64
		if closest != nil {
			byMeaning = closest[i]
		}
		ranked, err := r.rank(ctx, userID, text, byMeaning, limit)
		if err != nil {
			return found, err
		}
		for _, m := range ranked {
			if !seen[m.ID] && len(found) < limit {
				seen[m.ID] = true
				found = append(found, m)
			}
		}
	}
	return found, nil
}

// rank fuses the FTS ranking of text with the memories closest to it by meaning.
func (r Recall) rank(ctx context.Context, userID int64, text string, byMeaning []int64, limit int) ([]dom.Memory, error) {
	hits, err := r.Memories.Search(ctx, userID, dom.SearchTerms(text), "", 0, limit)
	if err != nil {
		return nil, err
	}
	memories := map[int64]dom.Memory{}
	byWords := make([]int64, len(hits))
	for i, m := range hits {
		memories[m.ID], byWords[i] = m, m.ID
	}
	var ranked []dom.Memory
	for _, id := range fuse(byWords, byMeaning) {
		m, ok := memories[id]
		if !ok {
			found, err := r.Memories.Get(ctx, userID, id)
			if err != nil {
				return nil, err
			}
			if found == nil {
				continue
			}
			m = *found
		}
		ranked = append(ranked, m)
	}
	return ranked[:min(len(ranked), limit)], nil
}
