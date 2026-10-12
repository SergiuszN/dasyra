package dom

import (
	"math"
	"testing"
)

func TestPriceCostSplitsTheCache(t *testing.T) {
	p := AIPrice{Input: 1, CacheRead: 0.1, CacheWrite: 1.25, Output: 4, PerMinute: 0.006}
	got := p.Cost(Usage{PromptTokens: 1_000_000, CachedTokens: 600_000, CacheWriteTokens: 100_000, CompletionTokens: 250_000})
	if want := 0.3 + 0.06 + 0.125 + 1.0; math.Abs(got-want) > 1e-9 {
		t.Fatalf("cost %v, want %v", got, want)
	}
	if got := p.Cost(Usage{AudioSeconds: 90}); math.Abs(got-0.009) > 1e-12 {
		t.Fatalf("audio %v", got)
	}
}
