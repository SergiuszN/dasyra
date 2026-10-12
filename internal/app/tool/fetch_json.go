package tool

import (
	"context"
	"errors"
	"fmt"

	"dasyra/internal/app/llm"
)

const (
	FetchJSONName  = "fetch_json"
	maxJSONResult  = 6000
	maxFilterError = 500
)

// FetchJSON reads a JSON API through a jq filter, so only the filtered result reaches the model:
// 165 KB of hero stats become a few hundred bytes. jq can only transform data, unlike scripts written by the model.
type FetchJSON struct {
	API JSONAPI
}

func (FetchJSON) Spec() llm.Tool {
	return Spec(FetchJSONName, `Fetch a public JSON API and return only what a jq filter extracts. First look for a "skill" memory with a working recipe. Use it for numbers and lists from APIs (stats, rates, schedules) instead of reading pages. Keep the output small: select fields, sort, limit. Try a shape probe first if you do not know the structure, e.g. `+"`.[0] | keys` or `keys`"+`. Once a recipe works for a recurring task, save url + filter to memory with category "skill" and reuse it next time.`,
		Str("url", "Public http(s) URL returning JSON").Required(),
		Str("jq", "jq filter applied to the response, e.g. [.[] | {name, value}] | .[:20]").Required(),
	)
}

func (t FetchJSON) Run(ctx context.Context, _ *Session, a Args) (string, error) {
	url, filter := a.String("url"), a.String("jq")
	if url == "" || filter == "" {
		return "Error: url and jq are required.", nil
	}

	out, err := t.API.Query(ctx, url, filter)
	var filterErr *FilterError
	switch {
	case errors.Is(err, ErrNotPublic):
		return "Error: only public http(s) URLs are allowed.", nil
	case errors.As(err, &filterErr):
		return "Error from jq (fix the filter, or the response is not JSON - then use web_fetch): " + cut(filterErr.Message, maxFilterError, ""), nil
	case err != nil:
		return fmt.Sprintf("Error: %s", err), nil
	case out == "":
		return "Empty result.", nil
	}
	return cut(out, maxJSONResult, "\n…truncated, narrow the filter (fewer fields, .[:N])."), nil
}
