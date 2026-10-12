package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"

	"dasyra/internal/app/llm"
)

const (
	researchRounds     = 5
	researchThreshold  = 0.78
	researchFirstRound = 0.9 // one round is rarely enough for multi-part questions, the first one stops only when nearly certain
	researchMaxPages   = 12
	researchStagnation = 0.03
)

const evaluatePrompt = "Evaluate evidence quality and produce strict JSON with keys: answer, confidence, key_evidence_points, remaining_gaps, next_query_hint. The answer must cover every part of the question with the concrete facts found (names, dates, places). Confidence (0..1) is how well ALL parts are backed by evidence; an unanswered part keeps it below 0.6. Never claim something does not exist or is not happening just because the evidence does not mention it, list it as a gap instead. Pages are more reliable than search snippets. next_query_hint is a short search query for the most important gap, in the language most likely to find it (e.g. the local language for one country)."

// research runs several search rounds for a hard multi-part question until the evidence is good enough.
type research struct {
	WebSearch
	session  *Session
	question string

	used     []string
	evidence evidence
	snippets []SearchResult
}

type evaluation struct {
	Answer     string   `json:"answer"`
	Confidence float64  `json:"confidence"`
	Points     []string `json:"key_evidence_points"`
	Gaps       []string `json:"remaining_gaps"`
	NextQuery  string   `json:"next_query_hint"`
}

func (r research) run(ctx context.Context) string {
	query, stagnant, previous := r.question, 0, 0.0
	var final *evaluation
	for round := 1; round <= researchRounds && len(r.used) < researchMaxPages; round++ {
		results, err := r.Search.Search(ctx, query)
		if err != nil {
			if final == nil {
				return searchDownMsg
			}
			break
		}
		r.addSnippets(results[:min(len(results), 5)])

		sources := pickSources(results, r.usedSet(), min(3, researchMaxPages-len(r.used)), 8)
		if len(sources) == 0 {
			break
		}
		r.used = append(r.used, sources...)
		r.evidence = append(r.evidence, r.WebSearch.evidence(ctx, r.session, sources, r.question)...)

		final = r.evaluate(ctx)
		if final.Confidence-previous < researchStagnation {
			stagnant++
		} else {
			stagnant = 0
		}
		previous = final.Confidence

		threshold := researchThreshold
		if round == 1 {
			threshold = researchFirstRound
		}
		if final.Confidence >= threshold || stagnant >= 2 {
			break
		}
		query = r.nextQuery(final)
	}

	if final == nil {
		return "I couldn't collect enough web data to research that yet."
	}
	return r.report(final)
}

func (r *research) addSnippets(results []SearchResult) {
	for _, res := range results {
		if i := indexOf(r.snippets, res.URL); i >= 0 {
			r.snippets[i] = res
		} else {
			r.snippets = append(r.snippets, res)
		}
	}
}

func indexOf(results []SearchResult, url string) int {
	for i, r := range results {
		if r.URL == url {
			return i
		}
	}
	return -1
}

func (r research) usedSet() map[string]bool {
	set := map[string]bool{}
	for _, u := range r.used {
		set[u] = true
	}
	return set
}

func (r research) nextQuery(e *evaluation) string {
	if next := strings.TrimSpace(e.NextQuery); next != "" {
		return next
	}
	if next := strings.TrimSpace(r.question + " " + strings.Join(e.Gaps, " ")); next != "" {
		return next
	}
	return r.question
}

func (r research) evaluate(ctx context.Context) *evaluation {
	snippets := formatResults(r.snippets)
	reply, err := r.AI.Chat(llm.For(ctx, 0, llm.PurposeWebSearch), llm.Request{Model: llm.WebSearch, Messages: []llm.Message{
		llm.System(evaluatePrompt),
		llm.User("Question:\n" + r.question + "\n\nEvidence from pages:\n" + r.evidence.payload(20000) + "\n\nSearch snippets:\n" + snippets),
	}})

	e := evaluation{Confidence: 0.5}
	if err != nil || json.Unmarshal([]byte(jsonObject.FindString(reply.Text)), &e) != nil {
		return &evaluation{
			Answer:     r.synthesize(ctx, r.question, r.evidence, snippets),
			Confidence: math.Min(0.9, 0.45+float64(len(r.evidence))*0.07),
			NextQuery:  r.question,
		}
	}
	e.Answer, e.NextQuery = strings.TrimSpace(e.Answer), strings.TrimSpace(e.NextQuery)
	e.Confidence = math.Max(0, math.Min(1, e.Confidence))
	return &e
}

var jsonObject = regexp.MustCompile(`(?s)\{.*\}`)

func (r research) report(e *evaluation) string {
	lines := []string{e.Answer, "", fmt.Sprintf("Confidence: %d%%", int(math.Round(e.Confidence*100)))}
	lines = append(lines, bullets("Key evidence:", e.Points)...)
	lines = append(lines, bullets("Remaining gaps:", e.Gaps)...)
	sources := r.used
	if len(r.evidence) > 0 {
		sources = r.evidence.urls()
	}
	return strings.Join(append(lines, "", "Sources:", formatSources(sources)), "\n")
}

func bullets(title string, items []string) []string {
	if len(items) == 0 {
		return nil
	}
	lines := []string{"", title}
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return lines
}
