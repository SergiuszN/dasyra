package tool

import (
	"bytes"
	"context"
	"errors"

	"dasyra/internal/dom"
)

// The outside world the web and document tools need, implemented in arch.

type SearchResult struct {
	Title, URL, Snippet string
}

// Searcher returns ranked results, an error only when every engine failed.
type Searcher interface {
	Search(ctx context.Context, query string) ([]SearchResult, error)
}

// Page is the readable text of a page or an online PDF.
type Page struct {
	Text     string
	Document bool // came from a PDF
}

type PageReader interface {
	Read(ctx context.Context, url string) (Page, error)
}

type JSONAPI interface {
	// Query fetches url and returns what the jq filter extracts
	Query(ctx context.Context, url, filter string) (string, error)
}

// Files downloads what the user sent in the chat.
type Files interface {
	Download(ctx context.Context, fileID string) ([]byte, error)
}

// Sender delivers text to the user's chat as it is.
type Sender interface {
	Send(ctx context.Context, chatID int64, text string) error
}

// PDFText extracts the text layer of a PDF, "" for scans.
type PDFText interface {
	Extract(ctx context.Context, pdf []byte) string
}

// CookbookAPI is the user's recipe server, every call with their connection.
type CookbookAPI interface {
	// Search lists recipes by words and tags (all of them), all by name without words
	Search(ctx context.Context, c dom.Cookbook, query string, tags []string, offset, limit int) ([]dom.Recipe, error)
	Recipe(ctx context.Context, c dom.Cookbook, slug string) (*dom.Recipe, error) // nil when missing
	// Import makes the server read a recipe page, Create saves one written here; both return the new slug
	Import(ctx context.Context, c dom.Cookbook, url string) (string, error)
	Create(ctx context.Context, c dom.Cookbook, r dom.Recipe) (string, error)
	// Update changes the fields that are set (lists replace the old ones) and returns the slug, new after a rename
	Update(ctx context.Context, c dom.Cookbook, slug string, r dom.Recipe) (string, error)
	Plan(ctx context.Context, c dom.Cookbook, from, to string) ([]dom.PlanEntry, error)
	AddToPlan(ctx context.Context, c dom.Cookbook, e dom.PlanEntry) error
	RemoveFromPlan(ctx context.Context, c dom.Cookbook, id int64) error
	ShoppingLists(ctx context.Context, c dom.Cookbook) ([]dom.ShoppingList, error) // without items
	CreateList(ctx context.Context, c dom.Cookbook, name string) (dom.ShoppingList, error)
	ShoppingList(ctx context.Context, c dom.Cookbook, id string) (*dom.ShoppingList, error)
	AddToList(ctx context.Context, c dom.Cookbook, listID string, lines []string) error
	AddRecipeToList(ctx context.Context, c dom.Cookbook, listID, slug string) error
	CheckOff(ctx context.Context, c dom.Cookbook, itemID string) error
	RemoveItem(ctx context.Context, c dom.Cookbook, itemID string) error
}

// ErrNotPublic is returned for URLs on the internal network: a page can talk the model into anything.
var ErrNotPublic = errors.New("only public http(s) URLs are allowed")

// FilterError is a jq filter that failed, the model should fix it.
type FilterError struct{ Message string }

func (e *FilterError) Error() string { return e.Message }

func isPDF(data []byte) bool {
	head := data[:min(len(data), 1024)]
	return bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n\x00"), []byte("%PDF"))
}
