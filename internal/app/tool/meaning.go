package tool

import (
	"cmp"
	"context"
	"log/slog"
	"maps"
	"math"
	"slices"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// Embedder turns texts into vectors and names the model that made them.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float32, string, error)
}

// Meaning finds the user's records (memories, messages, notes) by meaning, in other words or another language.
// Vectors are made lazily: a search embeds a batch of new or changed records, the nightly run all of them.
type Meaning struct {
	Vectors dom.Vectors // nil turns search by meaning off
	Embed   Embedder
	Log     *slog.Logger
}

const (
	staleBatch = 100  // records embedded in one call
	embedRunes = 2000 // texts are cut to this before embedding
	// Similarity scales differ between models (0.25-0.45 for related memories with text-embedding-3-small, 0.6-0.7
	// with Gemini), so the cut is relative to the user's records: the best match has to stand out from the median
	// by minSpread, and a record counts when it is in the upper half of that spread
	minSpread = 0.08
	rrfK      = 60 // reciprocal rank fusion of the word and meaning rankings
)

// Closest returns for each text up to max ids of kind standing out by meaning, most similar first;
// nil when search by meaning is off or failed, the caller then has words alone.
func (m Meaning) Closest(ctx context.Context, userID int64, kind string, texts []string, max int) [][]int64 {
	if m.Vectors == nil || m.Embed == nil || len(texts) == 0 {
		return nil
	}
	ctx = llm.For(ctx, 0, llm.PurposeEmbedding)
	queries, model, err := m.Embed.Embed(ctx, cutAll(texts))
	if err != nil {
		m.Log.Error("Embedding failed, searching by words only", "error", err)
		return nil
	}
	if _, err := m.index(ctx, userID, kind, model); err != nil {
		m.Log.Error("Failed to embed records", "user_id", userID, "kind", kind, "error", err)
	}
	vectors, err := m.Vectors.All(ctx, userID, kind, model)
	if err != nil {
		m.Log.Error("Failed to read vectors", "user_id", userID, "kind", kind, "error", err)
		return nil
	}
	closest := make([][]int64, len(queries))
	for i, q := range queries {
		similarity := make(map[int64]float64, len(vectors))
		for id, v := range vectors {
			similarity[id] = cosine(q, v)
		}
		closest[i] = standingOut(similarity, max)
	}
	return closest
}

// IndexAll embeds every new or changed record of the user, for the nightly run.
func (m Meaning) IndexAll(ctx context.Context, userID int64) error {
	if m.Vectors == nil || m.Embed == nil {
		return nil
	}
	ctx = llm.For(ctx, userID, llm.PurposeEmbedding)
	_, model, err := m.Embed.Embed(ctx, []string{"."}) // which model the role runs on now
	if err != nil {
		return err
	}
	for _, kind := range []string{dom.KindMemory, dom.KindNote, dom.KindMessage} {
		for {
			n, err := m.index(ctx, userID, kind, model)
			if err != nil {
				return err
			}
			if n < staleBatch {
				break
			}
		}
	}
	return nil
}

// index embeds one batch of the records of kind that are new or changed since their vector was made.
func (m Meaning) index(ctx context.Context, userID int64, kind, model string) (int, error) {
	stale, err := m.Vectors.Stale(ctx, userID, kind, model, staleBatch)
	if err != nil || len(stale) == 0 {
		return 0, err
	}
	at := time.Now()
	texts := make([]string, len(stale))
	for i, r := range stale {
		texts[i] = r.Text
	}
	vectors, made, err := m.Embed.Embed(ctx, cutAll(texts))
	if err != nil {
		return 0, err
	}
	if made != model {
		return 0, nil // the role moved to another model in between, the next search catches up
	}
	byID := make(map[int64][]float32, len(stale))
	for i, r := range stale {
		byID[r.ID] = vectors[i]
	}
	return len(stale), m.Vectors.Save(ctx, userID, kind, model, at, byID)
}

// standingOut are up to max ids standing out by similarity, most similar first; none when nothing stands out
// from the median, the text is then about nothing in particular among the records.
func standingOut(similarity map[int64]float64, max int) []int64 {
	ids := slices.Collect(maps.Keys(similarity))
	if len(ids) == 0 {
		return nil
	}
	slices.SortFunc(ids, func(a, b int64) int { return cmp.Compare(similarity[b], similarity[a]) })
	best, median := similarity[ids[0]], similarity[ids[len(ids)/2]]
	spread := best - median
	if spread < minSpread {
		return nil
	}
	n := 0
	for n < min(len(ids), max) && similarity[ids[n]] >= best-spread/2 {
		n++
	}
	return ids[:n]
}

// fuse orders ids by reciprocal rank fusion of rankings, each best first.
func fuse(rankings ...[]int64) []int64 {
	score := map[int64]float64{}
	var ids []int64
	for _, ranking := range rankings {
		for i, id := range ranking {
			if _, ok := score[id]; !ok {
				ids = append(ids, id)
			}
			score[id] += 1.0 / float64(rrfK+i)
		}
	}
	slices.SortStableFunc(ids, func(a, b int64) int { return cmp.Compare(score[b], score[a]) })
	return ids
}

func cutAll(texts []string) []string {
	out := make([]string, len(texts))
	for i, t := range texts {
		out[i] = cut(t, embedRunes, "")
	}
	return out
}

func cosine(a, b []float32) float64 {
	if len(a) != len(b) {
		return 0
	}
	var dot, na, nb float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		na += float64(a[i]) * float64(a[i])
		nb += float64(b[i]) * float64(b[i])
	}
	if na == 0 || nb == 0 {
		return 0
	}
	return dot / math.Sqrt(na*nb)
}
