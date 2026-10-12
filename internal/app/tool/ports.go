package tool

import (
	"bytes"
	"context"
	"errors"
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

// ErrNotPublic is returned for URLs on the internal network: a page can talk the model into anything.
var ErrNotPublic = errors.New("only public http(s) URLs are allowed")

// FilterError is a jq filter that failed, the model should fix it.
type FilterError struct{ Message string }

func (e *FilterError) Error() string { return e.Message }

func isPDF(data []byte) bool {
	head := data[:min(len(data), 1024)]
	return bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n\x00"), []byte("%PDF"))
}
