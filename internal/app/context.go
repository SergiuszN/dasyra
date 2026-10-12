package app

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/dom"
)

const (
	calendarLimit = 6
	memoryLimit   = 6
	todoLimit     = 40
	maxImages     = 4
	argsPreview   = 150
)

// prompt orders the context from the most stable to the most volatile part, so the provider reuses
// the cached prefix: cached input is ten times cheaper.
func (d *Dasyra) prompt(ctx context.Context, s *tool.Session, history dom.History, run Run) []llm.Message {
	u := s.User
	messages := []llm.Message{
		llm.System(systemPrompt(u, run.checkIn(), d.collections(ctx, u))).Cached(),
		llm.User(d.calendarContext(ctx, s)).Cached(),
	}
	if text := conversationContext(u, history); text != "" {
		messages = append(messages, llm.User(text).Cached())
	}
	if text := d.todoContext(ctx, u); text != "" {
		messages = append(messages, llm.User(text))
	}
	if text := d.memoryContext(ctx, s, history, run.MemoryQuery); text != "" {
		messages = append(messages, llm.User(text))
	}
	if !run.Proactive {
		if images := d.images(ctx, history); len(images) > 0 {
			messages = append(messages, llm.Message{Role: "user", Text: "Photos from the latest [photo] messages:", Images: images})
		}
		// Only for replies: in a check-in the hint made Dasyra "look at the results" and re-answer questions hours later
		if hint := d.recentToolsHint(ctx, u, history); hint != "" {
			messages = append(messages, llm.User(hint))
		}
	}
	now := u.Local(now())
	messages = append(messages, llm.User(fmt.Sprintf("Current date and time: %s (%s)", now.Format(dom.ShortTime), u.Timezone)))
	if run.Trigger != "" {
		messages = append(messages, llm.User(run.Trigger))
	}
	return messages
}

// collections are the names of the user's collections: they change rarely, so they live in the cached system prompt.
func (d *Dasyra) collections(ctx context.Context, u *dom.User) []string {
	collections, err := d.Notes.Collections(ctx, u.ID)
	if err != nil {
		d.Log.Error("Failed to read collections", "user_id", u.ID, "error", err)
	}
	names := make([]string, len(collections))
	for i, c := range collections {
		names[i] = c.Name
	}
	return names
}

// calendarContext lists the next events and the running series. Events shown with ids count as seen,
// so they can be changed without listing them first.
func (d *Dasyra) calendarContext(ctx context.Context, s *tool.Session) string {
	now := s.User.Local(now())
	upcoming, err := d.Events.Upcoming(ctx, s.User, now, calendarLimit)
	if err != nil {
		d.Log.Error("Failed to read upcoming events", "error", err)
	}
	recurring, err := d.Events.Recurring(ctx, s.User, now)
	if err != nil {
		d.Log.Error("Failed to read recurring events", "error", err)
	}
	return "Recent and upcoming events:\n" + listEvents(s, upcoming) + "\nRecurring events:\n" + listEvents(s, recurring)
}

func listEvents(s *tool.Session, events []dom.Event) string {
	if len(events) == 0 {
		return "None."
	}
	lines := make([]string, len(events))
	for i, e := range events {
		s.Events.Mark(e.ID)
		lines[i] = e.String()
	}
	return strings.Join(lines, "\n")
}

// todoContext is the open to-do list with what got done today, after the history: it changes with most replies.
func (d *Dasyra) todoContext(ctx context.Context, u *dom.User) string {
	if d.Todos == nil {
		return ""
	}
	local := u.Local(now())
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, local.Location())
	todos, err := d.Todos.Current(ctx, u.ID, midnight, todoLimit)
	if err != nil {
		d.Log.Error("Failed to read the to-do list", "user_id", u.ID, "error", err)
	}
	if len(todos) == 0 {
		return ""
	}
	today := local.Format(dom.DayLayout)
	lines := make([]string, len(todos))
	for i, t := range todos {
		lines[i] = t.Line(u, today)
	}
	return "To-do list of " + u.DisplayName() + " (open items and the ones done today):\n" + strings.Join(lines, "\n")
}

func conversationContext(u *dom.User, history dom.History) string {
	if len(history) == 0 {
		return ""
	}
	return "Current conversation (oldest to newest):\n" + chatLines(u, history.OldestFirst())
}

// chatLines shows messages oldest first, one per line, in the user's local time.
func chatLines(u *dom.User, messages []dom.Message) string {
	lines := make([]string, len(messages))
	for i, m := range messages {
		content := strings.ReplaceAll(strings.TrimSpace(m.Content), "\n", "\n  ")
		lines[i] = fmt.Sprintf("[%s] %s: %s", u.Local(m.CreatedAt).Format(dom.ShortTime), m.Role, content)
	}
	return strings.Join(lines, "\n")
}

// memoryContext finds memories by the messages not answered yet, then fills up by the exchange before them:
// a short or vague message ("what else is left?") finds little by itself. A query replaces both (a due event).
// Memories shown with their content count as seen, so they can be updated right away.
func (d *Dasyra) memoryContext(ctx context.Context, s *tool.Session, history dom.History, query string) string {
	queries := []string{query}
	if query == "" {
		queries = searchTexts(history)
	}
	found, err := d.Recall.Find(ctx, s.User.ID, queries, memoryLimit)
	if err != nil {
		d.Log.Error("Failed to search memories", "user_id", s.User.ID, "error", err)
	}
	if len(found) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("Relevant memories (use if helpful): ")
	for _, m := range found {
		s.Memories.Mark(m.ID)
		fmt.Fprintf(&b, "[#%d %s] %s: %s. ", m.ID, m.Category, m.Name, m.Content)
	}
	return b.String()
}

// searchTexts are the user messages not answered yet, then the exchange before them: Dasyra's last reply with
// the user messages it answered.
func searchTexts(history dom.History) []string {
	texts := []string{""}
	for _, m := range history {
		if m.Role == dom.RoleAssistant {
			if len(texts) == 2 {
				break
			}
			texts = append(texts, "")
		}
		texts[len(texts)-1] += m.Content + "\n"
	}
	return texts
}

// images downloads the photos of the messages not answered yet, oldest first.
func (d *Dasyra) images(ctx context.Context, history dom.History) []string {
	var fileIDs []string
	for _, m := range slices.Backward(history.Unanswered()) {
		if m.ImageFileID != "" {
			fileIDs = append(fileIDs, m.ImageFileID)
		}
	}
	var urls []string
	for _, id := range fileIDs[:min(len(fileIDs), maxImages)] {
		data, err := d.Chat.Download(ctx, id)
		if err != nil {
			d.Log.Error("Failed to download a photo", "error", err)
			continue
		}
		urls = append(urls, "data:"+http.DetectContentType(data)+";base64,"+base64.StdEncoding.EncodeToString(data))
	}
	return urls
}

// recentToolsHint points at the tool results behind a recent reply while it is in the visible history,
// so a follow-up question does not repeat the same searches.
func (d *Dasyra) recentToolsHint(ctx context.Context, u *dom.User, history dom.History) string {
	calls, err := d.ToolCalls.LatestRun(ctx, u.ID)
	if err != nil || len(calls) == 0 {
		return ""
	}
	reply := history.Contains(calls[0].ReplyID)
	if reply == nil {
		return ""
	}

	used := make([]string, len(calls))
	for i, c := range calls {
		used[i] = c.Tool + " " + strimWidth(c.Args, argsPreview)
	}
	// A plain log line: "you used save_event" made Dasyra confirm the event again in reply to a "thanks",
	// and an instruction like "do not repeat it" got answered as if the user wrote it
	return fmt.Sprintf("Tool log (not a message from %s): your reply at %s, already sent, used %s. Their full results are in %s.",
		u.DisplayName(), u.Local(reply.CreatedAt).Format(dom.ShortTime), strings.Join(used, "; "), tool.RecentResultsName)
}

// strimWidth cuts text to width runes with an ellipsis included.
func strimWidth(text string, width int) string {
	runes := []rune(text)
	if len(runes) <= width {
		return text
	}
	return string(runes[:width-1]) + "…"
}
