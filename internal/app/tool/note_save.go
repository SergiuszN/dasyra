package tool

import (
	"context"
	"fmt"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// SaveNote files a note into a collection or changes one. The model writes the text itself, so it can tidy
// the formatting of whatever the user sent.
type SaveNote struct {
	Notes dom.Notes
	IDs   IDs
}

func (SaveNote) Spec() llm.Tool {
	return Spec("save_note", "Save a note into a collection (a recipe, a gift idea, a film, a place), or change a saved note by its id. Save the whole text, never a summary. For a link, web_fetch it first asking for the full text word for word (every amount, every step), then save it with the link as source",
		Int("id", "Id of a saved note to change, leave empty for a new note"),
		Str("collection", "Short general collection name in the user's language (a cookbook, not \"pie recipes\"), reuse an existing one when it fits. Required for a new note"),
		Str("title", "Short title to recognize the note by. Required for a new note"),
		Str("tags", "Comma separated words to find it by: kind, main ingredient, person, city, genre"),
		Str("content", "The complete text, neatly formatted, with every detail kept (amounts, steps, links). Required for a new note"),
		Str("source", "Link or where it came from"),
	)
}

func (t SaveNote) Run(ctx context.Context, s *Session, a Args) (string, error) {
	content := a.String("content")
	collections, err := t.Notes.Collections(ctx, s.User.ID)
	if err != nil {
		return "", err
	}
	collection := dom.CollectionName(collections, a.String("collection"))

	if id := a.Int("id"); id > 0 {
		return t.update(ctx, s, id, collection, content, a)
	}
	title := a.String("title")
	if collection == "" || title == "" || content == "" {
		return "Error: a new note needs collection, title and content.", nil
	}
	if same, err := t.sameTitle(ctx, s.User.ID, collection, title); err != nil || same != nil {
		if same != nil {
			return fmt.Sprintf("Error: %q already has note #%d %q. Pass its id to change it, or use another title.", collection, same.ID, same.Title), nil
		}
		return "", err
	}

	now := time.Now()
	n := &dom.Note{ID: t.IDs.Next(), Collection: collection, Title: title, Tags: a.String("tags"), Content: content,
		Source: a.String("source"), CreatedAt: now, UpdatedAt: now}
	if err := t.Notes.Insert(ctx, s.User.ID, n); err != nil {
		return "", err
	}
	s.Notes.Mark(n.ID)
	return fmt.Sprintf("Note #%d %q saved to %q (%d characters).", n.ID, n.Title, n.Collection, len([]rune(content))), nil
}

func (t SaveNote) update(ctx context.Context, s *Session, id int64, collection, content string, a Args) (string, error) {
	n, err := t.Notes.Get(ctx, s.User.ID, id)
	if err != nil {
		return "", err
	}
	if n == nil {
		return fmt.Sprintf("Error: note #%d not found.", id), nil
	}
	if content != "" {
		if !s.Notes.Has(id) {
			return fmt.Sprintf("Error: note #%d was not read in this session, read it with find_notes by its id before replacing its text.", id), nil
		}
		n.Content = content
	}
	if collection != "" {
		n.Collection = collection
	}
	for key, field := range map[string]*string{"title": &n.Title, "tags": &n.Tags, "source": &n.Source} {
		if a.Has(key) {
			*field = a.String(key)
		}
	}
	n.UpdatedAt = time.Now()
	if err := t.Notes.Update(ctx, s.User.ID, n); err != nil {
		return "", err
	}
	return fmt.Sprintf("Note #%d %q in %q updated.", n.ID, n.Title, n.Collection), nil
}

func (t SaveNote) sameTitle(ctx context.Context, userID int64, collection, title string) (*dom.Note, error) {
	notes, err := t.Notes.Find(ctx, userID, dom.SearchTerms(title), collection, 0, 50)
	if err != nil {
		return nil, err
	}
	for _, n := range notes {
		if strings.EqualFold(n.Title, title) {
			return &n, nil
		}
	}
	return nil, nil
}
