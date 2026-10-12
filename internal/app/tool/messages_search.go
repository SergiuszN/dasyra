package tool

import (
	"context"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

type SearchOldMessages struct {
	Messages dom.Messages
	Meaning  Meaning
}

// maxMessagesByMeaning join the word hits at most
const maxMessagesByMeaning = 5

func (SearchOldMessages) Spec() llm.Tool {
	return Spec("search_old_message", "Search older conversation messages by words and by meaning, in any language",
		Str("query", "What to look for: words or a short description").Required(),
		Int("limit", "Max items (default 10)"),
		Str("from", "Start datetime inclusive (Y-m-d H:i:s)"),
		Str("to", "End datetime inclusive (Y-m-d H:i:s)"),
	)
}

type messageRow struct {
	Role      dom.Role `json:"role"`
	Content   string   `json:"content"`
	CreatedAt string   `json:"created_at"`
}

type messagePage struct {
	Count    int          `json:"count"`
	Limit    int          `json:"limit"`
	From     *string      `json:"from"`
	To       *string      `json:"to"`
	Message  string       `json:"message,omitempty"`
	Messages []messageRow `json:"messages,omitempty"`
}

func (t SearchOldMessages) Run(ctx context.Context, s *Session, a Args) (string, error) {
	query, limit := a.String("query"), a.IntOr("limit", 10)
	if query == "" {
		return "Error: query is required", nil
	}
	from, errFrom := parseOptional(s, a, "from")
	to, errTo := parseOptional(s, a, "to")
	if errFrom != nil || errTo != nil {
		return "Error: invalid date format for from/to", nil
	}
	if from != nil && to != nil && from.After(*to) {
		return "Error: from must be less than or equal to to", nil
	}

	page := messagePage{Limit: limit, From: formatTime(from), To: formatTime(to)}
	messages, err := t.search(ctx, s.User.ID, query, limit, from, to)
	if err != nil {
		return "", err
	}
	if len(messages) == 0 {
		page.Message = "No messages found matching the criteria."
		return toJSON(page), nil
	}

	for _, m := range messages {
		page.Messages = append(page.Messages, messageRow{m.Role, m.Content, s.User.Local(m.CreatedAt).Format(dom.DateTimeLayout)})
	}
	page.Count = len(messages)
	return toJSON(page), nil
}

// search fuses the word hits with the messages closest by meaning that fall into the dates.
func (t SearchOldMessages) search(ctx context.Context, userID int64, query string, limit int, from, to *time.Time) ([]dom.Message, error) {
	hits, err := t.Messages.Search(ctx, userID, dom.SearchTerms(query), limit, from, to)
	if err != nil {
		return nil, err
	}
	var byMeaning []int64
	if closest := t.Meaning.Closest(ctx, userID, dom.KindMessage, []string{query}, maxMessagesByMeaning); closest != nil {
		byMeaning = closest[0]
	}
	extra, err := t.Messages.Some(ctx, userID, byMeaning)
	if err != nil {
		return nil, err
	}
	messages := map[int64]dom.Message{}
	byWords := make([]int64, len(hits))
	for i, m := range hits {
		messages[m.ID], byWords[i] = m, m.ID
	}
	for _, m := range extra {
		if (from == nil || !m.CreatedAt.Before(*from)) && (to == nil || !m.CreatedAt.After(*to)) {
			messages[m.ID] = m
		}
	}
	var found []dom.Message
	for _, id := range fuse(byWords, byMeaning) {
		if m, ok := messages[id]; ok && len(found) < limit {
			found = append(found, m)
		}
	}
	return found, nil
}
