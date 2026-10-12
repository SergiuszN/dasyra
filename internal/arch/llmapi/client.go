// Package llmapi talks to the models through OpenRouter or any OpenAI-compatible API, routes each role to the
// provider set in the admin panel and records the cost of every call.
package llmapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

const (
	openRouterURL = "https://openrouter.ai/api/v1"
	// A stuck request is retried once, better than the user waiting five minutes
	chatTimeout = 90 * time.Second
)

var retryPause = 3 * time.Second

// Client is one provider. The OpenRouter kind sends its extras (provider sorting, reasoning, cache marks for
// Anthropic models) and gets the cost back; the OpenAI kind sends only the standard fields and reports no cost,
// so it is counted from the model's price set in the panel.
type Client struct {
	Name       string // in the logs
	Kind       string // dom.ProviderOpenRouter when empty
	Key        string
	ProviderID int64          // 0 for OpenRouter from .env
	Prices     dom.AISettings // prices of the provider's models, nil leaves a missing cost at 0
	Usage      dom.UsageLog
	Log        *slog.Logger
	BaseURL    string // empty for OpenRouter itself
	HTTP       *http.Client
}

func (c *Client) Chat(ctx context.Context, r llm.Request) (llm.Message, error) {
	m, err := c.try(ctx, r)
	if err == nil || ctx.Err() != nil {
		return m, err
	}
	c.Log.Warn("AI call failed, retrying", "provider", c.name(), "error", err)
	select {
	case <-time.After(retryPause):
	case <-ctx.Done():
		return m, ctx.Err()
	}
	return c.try(ctx, r)
}

func (c *Client) try(ctx context.Context, r llm.Request) (llm.Message, error) {
	body, err := json.Marshal(encode(r, c.openRouter()))
	if err != nil {
		return llm.Message{}, err
	}
	c.Log.Debug("AI request", "provider", c.name(), "model", r.Model.ID, "body", string(body))
	ctx, cancel := context.WithTimeout(ctx, chatTimeout)
	defer cancel()

	var resp response
	if err := c.post(ctx, "/chat/completions", "application/json", bytes.NewReader(body), &resp); err != nil {
		return llm.Message{}, err
	}
	if resp.Usage != nil {
		c.record(ctx, r.Model.ID, *resp.Usage)
	}
	m, ok := decode(resp)
	if !ok {
		return m, errors.New("invalid response from AI: no message")
	}
	return m, nil
}

func (c *Client) post(ctx context.Context, path, contentType string, body io.Reader, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path), body)
	if err != nil {
		return err
	}
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	req.Header.Set("Content-Type", contentType)

	res, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error: %d - %s", res.StatusCode, raw)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("JSON decode error: %w", err)
	}
	return nil
}

// record stores the call in the cost accounting: it must never break a reply. A cost the provider did not report
// is counted from the model's price, when one is set.
func (c *Client) record(ctx context.Context, model string, u usage) {
	payer := llm.PayerOf(ctx)
	rec := u.record(model, payer)
	if rec.Cost == 0 && c.Prices != nil && c.ProviderID > 0 {
		price, err := c.Prices.Price(ctx, c.ProviderID, model)
		if err != nil {
			c.Log.Error("Failed to read the model price", "model", model, "error", err)
		}
		if price != nil {
			rec.Cost, rec.Estimated = price.Cost(rec), true
		}
	}
	if err := c.Usage.Record(context.WithoutCancel(ctx), rec); err != nil {
		c.Log.Error("Failed to record AI usage", "error", err)
	}
	c.Log.Info("AI usage", "provider", c.name(), "model", model, "purpose", payer.Purpose, "prompt", rec.PromptTokens,
		"cached", rec.CachedTokens, "completion", rec.CompletionTokens, "cost", rec.Cost, "estimated", rec.Estimated)
}

func (c *Client) url(path string) string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/") + path
	}
	return openRouterURL + path
}

func (c *Client) openRouter() bool { return c.Kind == "" || c.Kind == dom.ProviderOpenRouter }

func (c *Client) name() string {
	if c.Name != "" {
		return c.Name
	}
	return "OpenRouter"
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}
