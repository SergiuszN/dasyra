package dom

import "strings"

// Provider kinds: OpenRouter, and any API speaking the OpenAI chat completions format (OpenAI, GLM, DeepSeek, Groq,
// Ollama, vLLM, LiteLLM...).
const (
	ProviderOpenRouter = "openrouter"
	ProviderOpenAI     = "openai"
)

// AIProvider is an API the models are called through, added with its key in the admin panel.
type AIProvider struct {
	ID      int64
	Name    string
	Kind    string
	BaseURL string // empty for OpenRouter itself
	Key     string
}

// MaskedKey shows only the end of the key, the panel never prints a key in full.
func (p AIProvider) MaskedKey() string {
	if p.Key == "" {
		return "no key"
	}
	return Mask(p.Key)
}

// AIRole is what one role (Dasyra, reflection, web search...) runs on. ProviderID 0 is OpenRouter with the key
// from .env, an empty Model or Effort keeps the role's default.
type AIRole struct {
	Role       string
	ProviderID int64
	Model      string
	Effort     string
}

// Default reports whether the role changes nothing, so it need not be stored.
func (r AIRole) Default() bool {
	return r.ProviderID == 0 && strings.TrimSpace(r.Model) == "" && r.Effort == ""
}

// AIPrice is what a model costs at a provider that does not report the cost itself, in USD per million tokens
// and per minute of audio. Set in the admin panel, it turns the tokens of a call into an estimated cost.
type AIPrice struct {
	ProviderID int64
	Model      string
	Input      float64 // uncached input
	CacheRead  float64
	CacheWrite float64
	Output     float64 // reasoning included, as OpenAI-compatible APIs count it
	PerMinute  float64 // transcription
}

// Cost of a call: cached and cache-write tokens are part of the prompt tokens and priced apart.
func (p AIPrice) Cost(u Usage) float64 {
	uncached := max(u.PromptTokens-u.CachedTokens-u.CacheWriteTokens, 0)
	tokens := float64(uncached)*p.Input + float64(u.CachedTokens)*p.CacheRead + float64(u.CacheWriteTokens)*p.CacheWrite +
		float64(u.CompletionTokens)*p.Output
	return tokens/1e6 + u.AudioSeconds/60*p.PerMinute
}
