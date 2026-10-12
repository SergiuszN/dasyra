package web

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"dasyra/internal/app/tool"
)

const maxPageBytes = 20_000_000

// Pages reads web pages and online PDFs as plain text.
type Pages struct {
	PDF    tool.PDFText
	client *http.Client
}

// NewPages reads only public addresses unless allowPrivate (tests against local servers).
func NewPages(pdf tool.PDFText, allowPrivate bool) *Pages {
	return &Pages{PDF: pdf, client: newClient(30*time.Second, allowPrivate, true)}
}

func (p *Pages) Read(ctx context.Context, raw string) (tool.Page, error) {
	u, err := checkURL(raw)
	if err != nil {
		return tool.Page{}, err
	}
	body, contentType, err := get(ctx, p.client, u.String(), "", maxPageBytes)
	if err != nil {
		return tool.Page{}, err
	}

	if strings.Contains(contentType, "pdf") || isPDF(body) {
		return tool.Page{Text: p.PDF.Extract(ctx, body), Document: true}, nil
	}
	// Other binaries turn into garbage after stripping tags and their invalid UTF-8 breaks the prompt
	if contentType != "" && !readable(contentType) {
		return tool.Page{}, fmt.Errorf("not a text page: %s", contentType)
	}
	return tool.Page{Text: minimize(string(body))}, nil
}

func readable(contentType string) bool {
	return strings.Contains(contentType, "html") || strings.HasPrefix(contentType, "text/") ||
		strings.Contains(contentType, "json") || strings.Contains(contentType, "xml")
}

func isPDF(data []byte) bool {
	head := data[:min(len(data), 1024)]
	return bytes.HasPrefix(bytes.TrimLeft(head, " \t\r\n\x00\x0b"), []byte("%PDF"))
}

var errTooLarge = errors.New("response is too large")

// get downloads at most limit bytes; a non-2xx status is an error.
func get(ctx context.Context, client *http.Client, url, accept string, limit int64) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", browserAgent)
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", &statusError{resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > limit {
		return nil, "", errTooLarge
	}
	return body, strings.ToLower(resp.Header.Get("Content-Type")), nil
}

type statusError struct{ code int }

func (e *statusError) Error() string {
	if e.code >= 300 && e.code < 400 {
		return fmt.Sprintf("HTTP %d redirect, use the final URL", e.code)
	}
	return fmt.Sprintf("HTTP %d", e.code)
}
