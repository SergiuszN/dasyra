package tool

import (
	"context"
	"fmt"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// FindNotes shows titles only: the model picks from them and send_note delivers the text without retyping it.
type FindNotes struct {
	Notes   dom.Notes
	Meaning Meaning
}

// maxNotesByMeaning join the word hits at most
const maxNotesByMeaning = 5

func (FindNotes) Spec() llm.Tool {
	return Spec("find_notes", "Look into the user's collections. Without arguments lists the collections. With collection and/or query lists matching notes by title and tags. With id returns that note in full. When the user asks what they have, offer a few titles to choose from; give a note as it is with send_note, or read it by id here to write an adapted version (half the portion, only the shopping list)",
		Str("collection", "Collection name"),
		Str("query", "Words to search titles, tags and text by"),
		Int("id", "Note id to read in full"),
		Int("offset", "Offset (default 0)"),
		Int("limit", "Max items (default 30)"),
	)
}

type noteRow struct {
	ID         int64  `json:"id"`
	Collection string `json:"collection,omitempty"`
	Title      string `json:"title"`
	Tags       string `json:"tags,omitempty"`
	Content    string `json:"content,omitempty"`
	Source     string `json:"source,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

type notePage struct {
	Count      int       `json:"count"`
	Notes      []noteRow `json:"notes"`
	NextOffset *int      `json:"next_offset,omitempty"`
}

func (t FindNotes) Run(ctx context.Context, s *Session, a Args) (string, error) {
	if id := a.Int("id"); id > 0 {
		return t.read(ctx, s, id)
	}
	collection, query := a.String("collection"), a.String("query")
	if collection == "" && query == "" {
		return t.collections(ctx, s.User.ID)
	}
	if collection != "" {
		collections, err := t.Notes.Collections(ctx, s.User.ID)
		if err != nil {
			return "", err
		}
		collection = dom.CollectionName(collections, collection)
	}
	offset, limit := a.IntOr("offset", 0), a.IntOr("limit", 30)
	notes, err := t.find(ctx, s.User.ID, query, collection, offset, limit)
	if err != nil {
		return "", err
	}
	if len(notes) == 0 {
		return "No notes found.", nil
	}
	page := notePage{Count: len(notes), NextOffset: nextOffset(len(notes), offset, limit)}
	for _, n := range notes {
		row := noteRow{ID: n.ID, Title: n.Title, Tags: n.Tags}
		if collection == "" {
			row.Collection = n.Collection
		}
		page.Notes = append(page.Notes, row)
	}
	return toJSON(page), nil
}

func (t FindNotes) read(ctx context.Context, s *Session, id int64) (string, error) {
	n, err := t.Notes.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if n == nil {
		return fmt.Sprintf("Error: note #%d not found.", id), nil
	}
	s.Notes.Mark(n.ID)
	return toJSON(noteRow{n.ID, n.Collection, n.Title, n.Tags, n.Content, n.Source, n.UpdatedAt.Format(dom.ShortTime)}), nil
}

func (t FindNotes) collections(ctx context.Context, userID int64) (string, error) {
	collections, err := t.Notes.Collections(ctx, userID)
	if err != nil {
		return "", err
	}
	if len(collections) == 0 {
		return "No collections yet.", nil
	}
	type row struct {
		Name  string `json:"name"`
		Notes int    `json:"notes"`
	}
	rows := make([]row, len(collections))
	for i, c := range collections {
		rows[i] = row{c.Name, c.Count}
	}
	return toJSON(map[string]any{"collections": rows}), nil
}

// find lists the collection without a query, otherwise fuses the word hits with the notes closest by meaning
// (on the first page: meaning has no pages).
func (t FindNotes) find(ctx context.Context, userID int64, query, collection string, offset, limit int) ([]dom.Note, error) {
	terms := dom.SearchTerms(query)
	if query == "" || offset > 0 {
		if query != "" && len(terms) == 0 {
			return nil, nil
		}
		return t.Notes.Find(ctx, userID, terms, collection, offset, limit)
	}
	var hits []dom.Note
	if len(terms) > 0 {
		var err error
		if hits, err = t.Notes.Find(ctx, userID, terms, collection, 0, limit); err != nil {
			return nil, err
		}
	}
	notes := map[int64]dom.Note{}
	byWords := make([]int64, len(hits))
	for i, n := range hits {
		notes[n.ID], byWords[i] = n, n.ID
	}
	var byMeaning []int64
	if closest := t.Meaning.Closest(ctx, userID, dom.KindNote, []string{query}, maxNotesByMeaning); closest != nil {
		byMeaning = closest[0]
	}
	var found []dom.Note
	for _, id := range fuse(byWords, byMeaning) {
		n, ok := notes[id]
		if !ok {
			got, err := t.Notes.Get(ctx, userID, id)
			if err != nil {
				return nil, err
			}
			if got == nil || (collection != "" && got.Collection != collection) {
				continue
			}
			n = *got
		}
		if len(found) < limit {
			found = append(found, n)
		}
	}
	return found, nil
}
