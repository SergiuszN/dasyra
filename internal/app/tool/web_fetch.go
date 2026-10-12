package tool

import (
	"context"
	"errors"
	"log/slog"

	"dasyra/internal/app/llm"
)

const WebFetchName = "web_fetch"

type WebFetch struct {
	Pages  PageReader
	Reader TextReader
	Log    *slog.Logger
}

func (WebFetch) Spec() llm.Tool {
	return Spec(WebFetchName, "Read a web page or an online PDF and answer a question about it. Shops and stats sites often block bots or build their tables with JavaScript, so the page comes back empty or without the numbers: for prices, stats, rates or game meta look for a public JSON API (fetch_json) instead of reading such pages. A source that gives nothing is not retried: find another route, and tell the user about it once.",
		Str("url", "Page URL").Required(),
		Str("question", "What to find on the page").Required(),
	)
}

func (t WebFetch) Run(ctx context.Context, _ *Session, a Args) (string, error) {
	url := a.String("url")
	if url == "" {
		return "Error: URL is required to fetch web content.", nil
	}

	page, err := t.Pages.Read(ctx, url)
	switch {
	case errors.Is(err, ErrNotPublic):
		return "Error: only public http(s) URLs are allowed.", nil
	case page.Text == "" && page.Document:
		return "Error: this PDF has no text layer (a scan or images), it can't be read.", nil
	case page.Text == "":
		t.Log.Error("Failed to fetch web content", "url", url, "error", err)
		// Luna kept retrying the same blocked shops and stats sites every day
		return `Error: Unable to fetch content from the provided URL. The site likely blocks bots: do not retry it, try another source or a public API through fetch_json. If it is a source of a recurring task, note it in the "skill" memory of that task.`, nil
	}

	// Raw pages never go to Dasyra, without a question the page gets summarized
	if page.Document {
		return t.Reader.Answer(ctx, page.Text, a.String("question"), documentLimit, "document"), nil
	}
	return t.Reader.Answer(ctx, page.Text, a.String("question"), pageLimit, "page"), nil
}
