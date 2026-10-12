package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"dasyra/internal/app/tool"
)

const searchInterval = 4 * time.Second

var errSearchDown = errors.New("web search is unavailable: every engine failed")

// Search asks the private SearXNG first and ddgr when it fails. An error means the search itself is down,
// so the model doesn't read it as "nothing exists".
type Search struct {
	URL      string // SearXNG, empty to skip it
	Throttle *Throttle
	Fallback func(ctx context.Context, query string) ([]tool.SearchResult, error)
	Log      *slog.Logger
	client   *http.Client
}

func NewSearch(searxngURL string, log *slog.Logger) *Search {
	return &Search{
		URL:      strings.TrimRight(searxngURL, "/"),
		Throttle: &Throttle{Interval: searchInterval},
		Fallback: ddgr,
		Log:      log,
		client:   &http.Client{Timeout: 20 * time.Second},
	}
}

func (s *Search) Search(ctx context.Context, query string) ([]tool.SearchResult, error) {
	s.Log.Debug("web_search", "query", query)
	s.Throttle.Wait(ctx)
	if results, err := s.searxng(ctx, query); err == nil && len(results) > 0 {
		return results, nil
	} else if err != nil {
		s.Log.Error("SearXNG search failed", "query", query, "error", err)
	}

	s.Throttle.Wait(ctx)
	results, err := s.Fallback(ctx, query)
	if err != nil {
		s.Log.Error("ddgr search failed", "query", query, "error", err)
		return nil, errSearchDown
	}
	return results, nil
}

func (s *Search) searxng(ctx context.Context, query string) ([]tool.SearchResult, error) {
	if s.URL == "" {
		return nil, errors.New("SearXNG is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL+"/search?"+url.Values{"q": {query}, "format": {"json"}}.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Results      []map[string]any `json:"results"`
		Unresponsive any              `json:"unresponsive_engines"`
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	if len(body.Results) == 0 {
		s.Log.Error("SearXNG returned no results", "query", query, "unresponsive", body.Unresponsive)
	}
	return normalize(body.Results, "content"), nil
}

var jsonArrayTail = regexp.MustCompile(`(?s)(\[.*\])\s*$`)

func ddgr(ctx context.Context, query string) ([]tool.SearchResult, error) {
	out, err := exec.CommandContext(ctx, "ddgr", "--np", "--json", query).CombinedOutput()
	raw := string(out)
	// A rate limit looks like "[ERROR] HTTP Error 202: Accepted" followed by an empty array
	if strings.Contains(raw, "[ERROR]") || (err != nil && raw == "") {
		return nil, fmt.Errorf("ddgr: %v %s", err, raw)
	}
	if m := jsonArrayTail.FindStringSubmatch(raw); m != nil {
		raw = m[1]
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("ddgr output: %w", err)
	}
	return normalize(items, "abstract"), nil
}

var spaces = regexp.MustCompile(`\s+`)

func normalize(items []map[string]any, snippetKey string) []tool.SearchResult {
	results := []tool.SearchResult{}
	for _, item := range items {
		u := strings.TrimSpace(text(item["url"]))
		if !strings.HasPrefix(u, "http") {
			continue
		}
		results = append(results, tool.SearchResult{
			Title:   strings.TrimSpace(text(item["title"])),
			URL:     u,
			Snippet: strings.TrimSpace(spaces.ReplaceAllString(text(item[snippetKey]), " ")),
		})
	}
	return results
}

func text(v any) string {
	s, _ := v.(string)
	return s
}
