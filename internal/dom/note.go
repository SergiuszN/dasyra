package dom

import (
	"strings"
	"time"
)

// Note is something the user asked to keep as it is (a recipe, a gift idea, a place), filed in a collection.
// Unlike a memory it is never rewritten by reflection and never pulled into the prompt by itself.
type Note struct {
	ID         int64
	Collection string
	Title      string
	Tags       string
	Content    string
	Source     string // a link or where it came from
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Text is the note as the user gets it, the title only when the text does not open with it.
func (n Note) Text() string {
	content := strings.TrimSpace(n.Content)
	parts := []string{content}
	if !strings.HasPrefix(strings.ToLower(content), strings.ToLower(n.Title)) {
		parts = []string{n.Title, content}
	}
	if n.Source != "" {
		parts = append(parts, n.Source)
	}
	return strings.Join(parts, "\n\n")
}

// Collection is a catalog of notes with how many it holds.
type Collection struct {
	Name  string
	Count int
}

// CollectionName returns the existing collection matching name in any letter case, so "кулинарная книга"
// files into "Кулинарная книга" instead of starting a second one.
func CollectionName(existing []Collection, name string) string {
	for _, c := range existing {
		if strings.EqualFold(c.Name, name) {
			return c.Name
		}
	}
	return name
}
