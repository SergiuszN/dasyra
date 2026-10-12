package llmapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// Router sends each call to the provider, model and effort its role is set to in the admin panel. Settings are
// read on every call (two tiny queries next to a model call), so a change applies at once. A role without settings,
// or whose settings can't be read, runs on its defaults through Default: OpenRouter with the key from .env.
type Router struct {
	Settings dom.AISettings
	Default  *Client
	Log      *slog.Logger
}

func (r *Router) Chat(ctx context.Context, req llm.Request) (llm.Message, error) {
	c, m := r.route(ctx, req.Model)
	req.Model = m
	return c.Chat(ctx, req)
}

func (r *Router) Transcribe(ctx context.Context, audio []byte, fileName string) (string, error) {
	c, m := r.route(ctx, llm.Transcription)
	return c.transcribe(ctx, m.ID, audio, fileName)
}

// Embed returns a vector per text and the model that made them: vectors of different models do not compare.
func (r *Router) Embed(ctx context.Context, texts []string) ([][]float32, string, error) {
	c, m := r.route(ctx, llm.Embedding)
	vectors, err := c.embed(ctx, m.ID, texts)
	return vectors, m.ID, err
}

func (r *Router) route(ctx context.Context, m llm.Model) (*Client, llm.Model) {
	roles, err := r.Settings.Roles(ctx)
	if err != nil {
		r.Log.Error("Failed to read AI roles, using defaults", "error", err)
		return r.Default, m
	}
	role, ok := roles[m.Role]
	if !ok {
		return r.Default, m
	}
	if role.Model != "" {
		m.ID = role.Model
	}
	if role.Effort != "" {
		m.Effort = role.Effort
	}
	if role.ProviderID == 0 {
		return r.Default, m
	}
	p, err := r.Settings.Provider(ctx, role.ProviderID)
	if err != nil || p == nil {
		r.Log.Error("AI provider of a role is gone, using defaults", "role", m.Role, "provider_id", role.ProviderID, "error", err)
		return r.Default, m
	}
	return r.client(*p), m
}

func (r *Router) client(p dom.AIProvider) *Client {
	return &Client{Name: p.Name, Kind: p.Kind, Key: p.Key, BaseURL: p.BaseURL, ProviderID: p.ID, Prices: r.Settings,
		Usage: r.Default.Usage, Log: r.Default.Log, HTTP: r.Default.HTTP}
}

const checkTimeout = 15 * time.Second

// Check tells whether the provider answers with its key: OpenRouter's key info (also when it is added as
// OpenAI-compatible), the model list of the others, which some APIs show without a key too. Nothing is paid for.
func (r *Router) Check(ctx context.Context, p dom.AIProvider) error {
	c := r.client(p)
	path := "/models"
	if c.openRouter() || strings.Contains(c.url(""), "://openrouter.ai/") {
		path = "/key" // its model list is public and would accept any key
	}
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url(path), nil)
	if err != nil {
		return err
	}
	if c.Key != "" {
		req.Header.Set("Authorization", "Bearer "+c.Key)
	}
	res, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d %s", res.StatusCode, strings.TrimSpace(http.StatusText(res.StatusCode)))
	}
	return nil
}

// SuggestPrices looks the models up in OpenRouter's public catalog, which lists most models at their vendor's
// price: by the exact id, else by the name after the vendor ("glm-5.2" finds "z-ai/glm-5.2"). Only a hint for the
// panel, models not found are missing.
func (r *Router) SuggestPrices(ctx context.Context, models []string) (map[string]dom.AIPrice, error) {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Default.url("/models"), nil)
	if err != nil {
		return nil, err
	}
	res, err := r.Default.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", res.StatusCode)
	}
	var catalog struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
				CacheRead  string `json:"input_cache_read"`
				CacheWrite string `json:"input_cache_write"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&catalog); err != nil {
		return nil, err
	}

	out := map[string]dom.AIPrice{}
	for _, m := range models {
		var byName *dom.AIPrice
		for _, entry := range catalog.Data {
			price := dom.AIPrice{Model: m, Input: perMillion(entry.Pricing.Prompt), Output: perMillion(entry.Pricing.Completion),
				CacheRead: perMillion(entry.Pricing.CacheRead), CacheWrite: perMillion(entry.Pricing.CacheWrite)}
			if entry.ID == m {
				byName = &price
				break
			}
			if _, name, _ := strings.Cut(entry.ID, "/"); name == m && byName == nil {
				byName = &price
			}
		}
		if byName != nil {
			out[m] = *byName
		}
	}
	return out, nil
}

// perMillion turns OpenRouter's USD per token, a string, into USD per million tokens.
func perMillion(perToken string) float64 {
	v, err := strconv.ParseFloat(perToken, 64)
	if err != nil || v < 0 {
		return 0
	}
	return math.Round(v*1e6*1e6) / 1e6
}
