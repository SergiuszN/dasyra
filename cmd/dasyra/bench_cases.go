package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"regexp"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

type benchCase struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Group       string           `json:"group"`
	Mode        string           `json:"mode"`
	At          string           `json:"at"`
	User        benchUser        `json:"user"`
	History     []benchMessage   `json:"history,omitempty"`
	Message     string           `json:"message,omitempty"`
	Memories    []benchMemory    `json:"memories,omitempty"`
	Events      []benchEvent     `json:"events,omitempty"`
	Todos       []benchTodo      `json:"todos,omitempty"`
	Notes       []benchNote      `json:"notes,omitempty"`
	RecentTools []benchPastTools `json:"recent_tools,omitempty"`
	Mocks       []benchMock      `json:"mocks,omitempty"`
	Review      []string         `json:"review"`
	Source      string           `json:"source,omitempty"`
}

type benchUser struct {
	Name     string `json:"name"`
	Profile  string `json:"profile"`
	Timezone string `json:"timezone"`
}

type benchMessage struct {
	ID      int64    `json:"id,omitempty"`
	At      string   `json:"at,omitempty"`
	Role    dom.Role `json:"role"`
	Text    string   `json:"text"`
	CheckIn bool     `json:"check_in,omitempty"`
}

type benchMemory struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Category string `json:"category"`
	Content  string `json:"content"`
}

type benchEvent struct {
	ID       int64    `json:"id"`
	Title    string   `json:"title"`
	Start    string   `json:"start"`
	End      string   `json:"end,omitempty"`
	Location string   `json:"location,omitempty"`
	Repeat   []string `json:"repeat,omitempty"`
	Reminded string   `json:"reminded,omitempty"`
}

type benchTodo struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
	Day  string `json:"day,omitempty"`
	Done bool   `json:"done,omitempty"`
}

type benchNote struct {
	ID         int64  `json:"id"`
	Collection string `json:"collection"`
	Title      string `json:"title"`
	Tags       string `json:"tags,omitempty"`
	Content    string `json:"content"`
	Source     string `json:"source,omitempty"`
}

type benchPastTools struct {
	ReplyID int64       `json:"reply_id"`
	Calls   []benchCall `json:"calls"`
}

type benchMock struct {
	Tool   string            `json:"tool"`
	Match  map[string]string `json:"match,omitempty"`
	Result string            `json:"result"`
}

func (m benchMock) matches(args tool.Args) bool {
	for k, v := range m.Match {
		if args.String(k) != v {
			return false
		}
	}
	return true
}

var benchID = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,79}$`)

func benchEffort(s string) bool {
	return s == "" || s == "none" || s == "low" || s == "medium" || s == "high" || s == "xhigh"
}

func readBenchRoles(file string) (map[string]benchRole, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var roles map[string]benchRole
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&roles); err != nil {
		return nil, err
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("bench roles: expected one JSON object")
	}
	known := map[string]bool{}
	for _, m := range llm.Models {
		known[m.Role] = true
	}
	for role, cfg := range roles {
		if !known[role] || !benchEffort(cfg.Effort) {
			return nil, fmt.Errorf("bench roles: unknown role or effort: %s", role)
		}
	}
	return roles, nil
}

func readBenchCases(file, only string) ([]benchCase, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cases []benchCase
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&cases); err != nil {
		return nil, fmt.Errorf("bench cases: %w", err)
	}
	if err := dec.Decode(new(any)); err != io.EOF {
		return nil, errors.New("bench cases: expected one JSON array")
	}
	seen := map[string]bool{}
	for i := range cases {
		c := &cases[i]
		if c.Mode == "" {
			c.Mode = "chat"
		}
		if c.User.Timezone == "" {
			c.User.Timezone = "Europe/Warsaw"
		}
		if seen[c.ID] || !benchID.MatchString(c.ID) {
			return nil, fmt.Errorf("bench cases: invalid or duplicate id %q", c.ID)
		}
		seen[c.ID] = true
		if err := c.validate(); err != nil {
			return nil, fmt.Errorf("bench case %s: %w", c.ID, err)
		}
	}
	if only == "" {
		if len(cases) == 0 {
			return nil, errors.New("bench cases: empty suite")
		}
		return cases, nil
	}
	selected := map[string]bool{}
	for _, pattern := range strings.Split(only, ",") {
		pattern = strings.TrimSpace(pattern)
		matched := false
		for _, c := range cases {
			ok, err := path.Match(pattern, c.ID)
			if err != nil {
				return nil, err
			}
			if ok {
				selected[c.ID], matched = true, true
			}
		}
		if !matched {
			return nil, fmt.Errorf("bench: no cases match %q", pattern)
		}
	}
	var out []benchCase
	for _, c := range cases {
		if selected[c.ID] {
			out = append(out, c)
		}
	}
	return out, nil
}

func (c benchCase) time(raw string) (time.Time, error) {
	return (&dom.User{Timezone: c.User.Timezone}).ParseLocal(raw)
}

func (c benchCase) validate() error {
	if c.Title == "" || c.User.Name == "" || c.At == "" {
		return errors.New("title, user.name and at are required")
	}
	if !dom.ValidTimezone(c.User.Timezone) {
		return errors.New("invalid user timezone")
	}
	at, err := c.time(c.At)
	if err != nil {
		return err
	}
	switch c.Mode {
	case "chat":
		if strings.TrimSpace(c.Message) == "" {
			return errors.New("chat needs a message")
		}
	case "checkin", "reflection", "reminder":
		if c.Message != "" {
			return errors.New("only chat mode accepts message; put prior messages in history")
		}
	default:
		return errors.New("mode must be chat, checkin, reminder or reflection")
	}
	seen := map[int64]bool{}
	previous := at.Add(-time.Duration(len(c.History)+1) * time.Minute)
	for i, m := range c.History {
		id := m.ID
		if id == 0 {
			id = int64(i + 1)
		}
		if id < 1 || seen[id] || (i > 0 && id <= historyID(c.History[i-1], i-1)) {
			return errors.New("history ids must be positive, unique and increasing")
		}
		seen[id] = true
		if m.Role != dom.RoleUser && m.Role != dom.RoleAssistant || m.Text == "" {
			return errors.New("history needs user/assistant roles and nonempty text")
		}
		t := at.Add(-time.Duration(len(c.History)-i) * time.Minute)
		if m.At != "" {
			if t, err = c.time(m.At); err != nil {
				return err
			}
		}
		if t.After(at) || (i > 0 && t.Before(previous)) {
			return errors.New("history must be chronological and at or before the scenario")
		}
		previous = t
	}
	for _, past := range c.RecentTools {
		if !seen[past.ReplyID] {
			return errors.New("recent_tools.reply_id must refer to a history message")
		}
		for i, m := range c.History {
			if historyID(m, i) == past.ReplyID && m.Role != dom.RoleAssistant {
				return errors.New("recent_tools must refer to an assistant reply")
			}
		}
	}
	for _, m := range c.Memories {
		if m.Name == "" || m.Category == "" || m.Content == "" {
			return errors.New("memories need name, category and content")
		}
	}
	for _, n := range c.Notes {
		if n.Collection == "" || n.Title == "" || n.Content == "" {
			return errors.New("notes need collection, title and content")
		}
	}
	for _, t := range c.Todos {
		if t.Text == "" {
			return errors.New("todos need text")
		}
		if _, err := (&dom.User{Timezone: c.User.Timezone}).ParseDay(t.Day); err != nil {
			return err
		}
	}
	for _, ids := range [][]int64{
		seedIDs(c.Memories, func(m benchMemory) int64 { return m.ID }),
		seedIDs(c.Events, func(e benchEvent) int64 { return e.ID }),
		seedIDs(c.Todos, func(t benchTodo) int64 { return t.ID }),
		seedIDs(c.Notes, func(n benchNote) int64 { return n.ID }),
	} {
		seen := map[int64]bool{}
		for i, id := range ids {
			if id < 0 || seen[seedID(id, i)] {
				return errors.New("seed ids must be positive and unique within each record kind")
			}
			seen[seedID(id, i)] = true
		}
	}
	for _, e := range c.Events {
		if e.Title == "" || e.Start == "" {
			return errors.New("events need title and start")
		}
		for _, raw := range []string{e.Start, e.End, e.Reminded} {
			if raw != "" {
				if _, err := c.time(raw); err != nil {
					return err
				}
			}
		}
		for _, day := range e.Repeat {
			if dom.NormalizeRepeatDays([]string{day}) == "" {
				return fmt.Errorf("unknown repeat weekday %q", day)
			}
		}
	}
	if c.Mode == "reminder" {
		due := false
		for _, e := range c.Events {
			start, _ := c.time(e.Start)
			event := dom.Event{Start: start, RepeatDays: dom.NormalizeRepeatDays(e.Repeat)}
			if e.Reminded != "" {
				t, _ := c.time(e.Reminded)
				event.RemindedAt = &t
			}
			due = due || event.Due(at.Add(-30*time.Minute), at)
		}
		if !due {
			return errors.New("reminder needs an unreminded event due within 30 minutes")
		}
	}
	for _, m := range c.Mocks {
		if !externalBenchTool(m.Tool) || m.Result == "" {
			return errors.New("mocks need an external tool name and nonempty result")
		}
	}
	return nil
}

func seedIDs[T any](items []T, id func(T) int64) []int64 {
	out := make([]int64, len(items))
	for i, item := range items {
		out[i] = id(item)
	}
	return out
}

func historyID(m benchMessage, i int) int64 {
	if m.ID > 0 {
		return m.ID
	}
	return int64(i + 1)
}

func seedBench(ctx context.Context, db *sqlite.DB, c benchCase) (*dom.User, error) {
	at, _ := c.time(c.At)
	u := &dom.User{ID: dom.OwnerID, ChatID: 1000, Name: c.User.Name, Profile: c.User.Profile, Timezone: c.User.Timezone, CreatedAt: at}
	users := sqlite.Users{DB: db}
	if err := users.Update(ctx, u); err != nil {
		return nil, err
	}
	messages := sqlite.Messages{DB: db}
	lastID := int64(0)
	for i, m := range c.History {
		t := at.Add(-time.Duration(len(c.History)-i) * time.Minute)
		if m.At != "" {
			t, _ = c.time(m.At)
		}
		lastID = historyID(m, i)
		msg := &dom.Message{ID: lastID, Role: m.Role, Content: m.Text, CreatedAt: t, TelegramID: lastID, CheckIn: m.CheckIn}
		if err := messages.Insert(ctx, u.ID, msg); err != nil {
			return nil, err
		}
	}
	if c.Message != "" {
		msg := &dom.Message{ID: lastID + 1, Role: dom.RoleUser, Content: c.Message, CreatedAt: at, TelegramID: lastID + 1}
		if err := messages.Insert(ctx, u.ID, msg); err != nil {
			return nil, err
		}
	}
	for i, m := range c.Memories {
		id := seedID(m.ID, i)
		if err := (sqlite.Memories{DB: db}).Insert(ctx, u.ID, &dom.Memory{ID: id, Name: m.Name, Category: m.Category, Content: m.Content, CreatedAt: at.Add(-24 * time.Hour), UpdatedAt: at.Add(-time.Hour)}); err != nil {
			return nil, err
		}
	}
	for i, e := range c.Events {
		start, _ := c.time(e.Start)
		event := &dom.Event{ID: seedID(e.ID, i), Title: e.Title, Start: start, Location: e.Location, RepeatDays: dom.NormalizeRepeatDays(e.Repeat)}
		if e.End != "" {
			t, _ := c.time(e.End)
			event.End = &t
		}
		if e.Reminded != "" {
			t, _ := c.time(e.Reminded)
			event.RemindedAt = &t
		}
		if err := (sqlite.Events{DB: db}).Insert(ctx, u, event); err != nil {
			return nil, err
		}
	}
	for i, t := range c.Todos {
		day, err := u.ParseDay(t.Day)
		if err != nil {
			return nil, err
		}
		todo := &dom.Todo{ID: seedID(t.ID, i), Text: t.Text, Day: day, CreatedAt: at.Add(-24 * time.Hour)}
		if t.Done {
			done := at.Add(-time.Minute)
			todo.DoneAt = &done
		}
		if err := (sqlite.Todos{DB: db}).Insert(ctx, u.ID, todo); err != nil {
			return nil, err
		}
	}
	for i, n := range c.Notes {
		note := &dom.Note{ID: seedID(n.ID, i), Collection: n.Collection, Title: n.Title, Tags: n.Tags, Content: n.Content, Source: n.Source, CreatedAt: at.Add(-24 * time.Hour), UpdatedAt: at.Add(-24 * time.Hour)}
		if err := (sqlite.Notes{DB: db}).Insert(ctx, u.ID, note); err != nil {
			return nil, err
		}
	}
	for _, p := range c.RecentTools {
		var calls []dom.ToolCall
		for _, call := range p.Calls {
			calls = append(calls, dom.ToolCall{Tool: call.Tool, Args: call.Args, Result: call.Result})
		}
		if err := (sqlite.ToolCalls{DB: db}).SaveRun(ctx, u.ID, p.ReplyID, calls); err != nil {
			return nil, err
		}
	}
	return u, nil
}

func seedID(id int64, i int) int64 {
	if id > 0 {
		return id
	}
	return int64(i + 1)
}

type benchRow map[string]any
type benchState map[string][]benchRow

func readBenchState(ctx context.Context, db *sqlite.DB) (benchState, error) {
	queries := map[string]string{
		"profile":  "SELECT id, name, profile, timezone FROM users WHERE id = 1",
		"memories": "SELECT id, name, category, content FROM memories WHERE user_id = 1 ORDER BY id",
		"events":   "SELECT id, event, start_time, end_time, location, repeat_interval, reminded_at FROM calendar_events WHERE user_id = 1 ORDER BY id",
		"todos":    "SELECT id, text, day, done_at FROM todos WHERE user_id = 1 ORDER BY id",
		"notes":    "SELECT id, collection, title, tags, content, source FROM notes WHERE user_id = 1 ORDER BY id",
	}
	state := benchState{}
	for table, query := range queries {
		rows, err := benchRows(ctx, db, query)
		if err != nil {
			return nil, err
		}
		state[table] = rows
	}
	return state, nil
}

func benchRows(ctx context.Context, db *sqlite.DB, query string) ([]benchRow, error) {
	rows, err := db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []benchRow{}
	for rows.Next() {
		values, pointers := make([]any, len(columns)), make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		row := benchRow{}
		for i, name := range columns {
			if bytes, ok := values[i].([]byte); ok {
				values[i] = string(bytes)
			}
			row[name] = values[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
