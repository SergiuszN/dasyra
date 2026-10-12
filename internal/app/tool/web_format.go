package tool

import (
	"fmt"
	"slices"
	"strings"
)

// page is what a fetched source said about the question.
type page struct {
	url, content string
}

type evidence []page

func (e evidence) urls() []string {
	var urls []string
	for _, p := range e {
		if !slices.Contains(urls, p.url) {
			urls = append(urls, p.url)
		}
	}
	return urls
}

// payload joins the pages for the model, at most 3000 characters per page and budget in total.
func (e evidence) payload(budget int) string {
	var chunks []string
	used := 0
	for _, p := range e {
		content := strings.TrimSpace(p.content)
		if content == "" {
			continue
		}
		chunk := "Source: " + p.url + "\n" + cut(content, 3000, "")
		if used+len(chunk)+8 > budget {
			break
		}
		chunks = append(chunks, chunk)
		used += len(chunk) + 8
	}
	if len(chunks) == 0 {
		return "(no pages could be read)"
	}
	return strings.Join(chunks, "\n\n---\n\n")
}

func formatResults(results []SearchResult) string {
	lines := make([]string, len(results))
	for i, r := range results {
		lines[i] = fmt.Sprintf("%d. %s\n   %s", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			lines[i] += "\n   " + cut(r.Snippet, 300, "")
		}
	}
	return strings.Join(lines, "\n")
}

func formatSources(urls []string) string {
	var lines []string
	for _, u := range urls {
		if !slices.Contains(lines, u) {
			lines = append(lines, u)
		}
	}
	for i, u := range lines {
		lines[i] = fmt.Sprintf("%d. %s", i+1, u)
	}
	return strings.Join(lines, "\n")
}
