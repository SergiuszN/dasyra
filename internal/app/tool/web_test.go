package tool_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/dom"
)

type searcher struct {
	results []tool.SearchResult
	err     error
	queries []string
}

func (s *searcher) Search(_ context.Context, q string) ([]tool.SearchResult, error) {
	s.queries = append(s.queries, q)
	return s.results, s.err
}

type pages map[string]tool.Page

func (p pages) Read(_ context.Context, url string) (tool.Page, error) {
	if url == "http://10.0.0.1/" {
		return tool.Page{}, tool.ErrNotPublic
	}
	return p[url], nil
}

// replies answers every call with the next text and counts the calls per model.
type replies struct {
	texts []string
	calls map[string]int
}

func (r *replies) Chat(_ context.Context, req llm.Request) (llm.Message, error) {
	if r.calls == nil {
		r.calls = map[string]int{}
	}
	r.calls[req.Model.ID]++
	text := r.texts[0]
	if len(r.texts) > 1 {
		r.texts = r.texts[1:]
	}
	return llm.Message{Role: "assistant", Text: text}, nil
}

var session = tool.NewSession(&dom.User{ID: 1, Timezone: "Europe/Warsaw"})

func results(n int) []tool.SearchResult {
	var out []tool.SearchResult
	for i := range n {
		out = append(out, tool.SearchResult{Title: fmt.Sprint("T", i), URL: fmt.Sprintf("https://site%d.pl/page", i), Snippet: "snippet"})
	}
	return out
}

func webSearch(s *searcher, ai *replies, p pages) tool.WebSearch {
	fetch := tool.WebFetch{Pages: p, Reader: tool.TextReader{AI: ai, Log: discard}, Log: discard}
	return tool.WebSearch{Search: s, Fetch: fetch, AI: ai, Log: discard}
}

func TestResultsModeListsTopEight(t *testing.T) {
	got, _ := webSearch(&searcher{results: results(10)}, &replies{}, nil).Run(ctx, session, tool.Args{"query": "x"})
	if !strings.HasPrefix(got, "1. T0\n   https://site0.pl/page\n   snippet") || !strings.Contains(got, "8. T7") || strings.Contains(got, "9. T8") {
		t.Fatalf("got %q", got)
	}
}

func TestSearchDownIsNotNothingFound(t *testing.T) {
	got, _ := webSearch(&searcher{err: errors.New("banned")}, &replies{}, nil).Run(ctx, session, tool.Args{"query": "x"})
	if !strings.Contains(got, "search is down, not that nothing was found") {
		t.Fatalf("got %q", got)
	}
}

func TestAnswerModeReadsOnePagePerSiteAndSkipsVideo(t *testing.T) {
	r := []tool.SearchResult{
		{URL: "https://youtube.com/watch?v=1"}, {URL: "https://a.pl/1"}, {URL: "https://a.pl/2"}, {URL: "https://b.pl/1"},
	}
	p := pages{"https://a.pl/1": {Text: "menu A"}, "https://b.pl/1": {Text: "menu B"}}
	ai := &replies{texts: []string{"read", "read", "The market is open"}}
	got, _ := webSearch(&searcher{results: r}, ai, p).Run(ctx, session, tool.Args{"query": "farmers market", "mode": "answer"})
	if got != "The market is open\n\nSources:\n1. https://a.pl/1\n2. https://b.pl/1" {
		t.Fatalf("got %q", got)
	}
}

func TestResearchStopsWhenConfident(t *testing.T) {
	ai := &replies{texts: []string{"page text", "page text", "page text",
		`{"answer":"The market opens on 18 Nov","confidence":0.95,"key_evidence_points":["city.example"],"remaining_gaps":[],"next_query_hint":""}`}}
	p := pages{}
	for _, r := range results(3) {
		p[r.URL] = tool.Page{Text: "content"}
	}
	s := &searcher{results: results(3)}
	got, _ := webSearch(s, ai, p).Run(ctx, session, tool.Args{"query": "farmers market", "mode": "research"})
	if len(s.queries) != 1 || !strings.HasPrefix(got, "The market opens on 18 Nov\n\nConfidence: 95%\n\nKey evidence:\n- city.example") {
		t.Fatalf("queries %v, got %q", s.queries, got)
	}
}

func TestWebFetchErrors(t *testing.T) {
	fetch := tool.WebFetch{Pages: pages{"https://scan.pl/a.pdf": {Document: true}}, Reader: tool.TextReader{AI: &replies{texts: []string{"x"}}, Log: discard}, Log: discard}
	cases := map[string]string{
		"http://10.0.0.1/":      "Error: only public http(s) URLs are allowed.",
		"https://scan.pl/a.pdf": "Error: this PDF has no text layer (a scan or images), it can't be read.",
		"https://blocked.pl/":   "Error: Unable to fetch content",
	}
	for url, want := range cases {
		if got, _ := fetch.Run(ctx, session, tool.Args{"url": url}); !strings.HasPrefix(got, want) {
			t.Errorf("%s: got %q", url, got)
		}
	}
}

type api struct {
	out string
	err error
}

func (a api) Query(context.Context, string, string) (string, error) { return a.out, a.err }

func TestFetchJSONResults(t *testing.T) {
	cases := []struct {
		api  api
		want string
	}{
		{api{out: `{"usd":4.1}`}, `{"usd":4.1}`},
		{api{}, "Empty result."},
		{api{err: &tool.FilterError{Message: "jq: error: syntax"}}, "Error from jq (fix the filter, or the response is not JSON - then use web_fetch): jq: error: syntax"},
		{api{err: fmt.Errorf("dial: %w", tool.ErrNotPublic)}, "Error: only public http(s) URLs are allowed."},
		{api{err: errors.New("HTTP 404")}, "Error: HTTP 404"},
		{api{out: strings.Repeat("1", 7000)}, strings.Repeat("1", 6000) + "\n…truncated, narrow the filter (fewer fields, .[:N])."},
	}
	for _, c := range cases {
		if got, _ := (tool.FetchJSON{API: c.api}).Run(ctx, session, tool.Args{"url": "https://x.pl", "jq": "."}); got != c.want {
			t.Errorf("got %q, want %q", got, c.want)
		}
	}
}
