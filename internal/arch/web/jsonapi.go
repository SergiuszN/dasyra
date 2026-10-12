package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/itchyny/gojq"

	"dasyra/internal/app/tool"
)

const (
	maxJSONBytes  = 10_000_000
	maxJQOutput   = 6000 * 4 // a huge output is cut instead of collected whole
	filterTimeout = 10 * time.Second
)

// JSON reads public JSON APIs through a jq filter. Redirects are not followed: the model gets the final URL to use.
type JSON struct {
	client *http.Client
}

// NewJSON reads only public addresses unless allowPrivate (tests against local servers).
func NewJSON(allowPrivate bool) *JSON {
	return &JSON{client: newClient(30*time.Second, allowPrivate, false)}
}

func (j *JSON) Query(ctx context.Context, raw, filter string) (string, error) {
	u, err := checkURL(raw)
	if err != nil {
		return "", err
	}
	body, _, err := get(ctx, j.client, u.String(), "application/json", maxJSONBytes)
	var status *statusError
	switch {
	case errors.Is(err, tool.ErrNotPublic):
		return "", err
	case errors.As(err, &status):
		return "", status
	case err != nil:
		return "", fmt.Errorf("request failed (%v)", err)
	}
	return runJQ(ctx, filter, body)
}

// runJQ works like `jq -c filter`: every input value goes through the filter, one compact result per line.
func runJQ(ctx context.Context, filter string, body []byte) (string, error) {
	query, err := gojq.Parse(filter)
	if err != nil {
		return "", &tool.FilterError{Message: err.Error()}
	}
	ctx, cancel := context.WithTimeout(ctx, filterTimeout)
	defer cancel()

	var out strings.Builder
	decoder := json.NewDecoder(bytes.NewReader(body))
	for out.Len() < maxJQOutput {
		var input any
		if err := decoder.Decode(&input); err == io.EOF {
			break
		} else if err != nil {
			return "", &tool.FilterError{Message: "parse error: " + err.Error()}
		}
		if err := collect(ctx, query.RunWithContext(ctx, input), &out); err != nil {
			return "", err
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func collect(ctx context.Context, results gojq.Iter, out *strings.Builder) error {
	for out.Len() < maxJQOutput {
		v, ok := results.Next()
		if !ok {
			return nil
		}
		if err, isErr := v.(error); isErr {
			var halt *gojq.HaltError
			if errors.As(err, &halt) && halt.Value() == nil {
				return nil
			}
			if ctx.Err() != nil {
				return &tool.FilterError{Message: "filter timed out"}
			}
			return &tool.FilterError{Message: err.Error()}
		}
		line, err := gojq.Marshal(v)
		if err != nil {
			return &tool.FilterError{Message: err.Error()}
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return nil
}
