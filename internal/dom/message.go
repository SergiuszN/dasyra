package dom

import (
	"slices"
	"time"
)

type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one row of the endless chat. A reply split into several Telegram messages is still one row.
type Message struct {
	ID             int64
	Role           Role
	Content        string
	TelegramID     int64 // 0 for Dasyra's replies
	CreatedAt      time.Time
	ImageFileID    string
	DocumentFileID string
	CheckIn        bool
}

// History is a slice of messages, newest first, as the chat window is read.
type History []Message

func (h History) Newest() *Message {
	if len(h) == 0 {
		return nil
	}
	return &h[0]
}

// LastFromUser is the newest user message: in a follow-up it can sit before the previous reply.
func (h History) LastFromUser() *Message {
	for i := range h {
		if h[i].Role == RoleUser {
			return &h[i]
		}
	}
	return nil
}

// Unanswered are the user messages after Dasyra's last reply, newest first.
func (h History) Unanswered() History {
	for i, m := range h {
		if m.Role == RoleAssistant {
			return h[:i]
		}
	}
	return h
}

// OldestFirst is the history in reading order.
func (h History) OldestFirst() []Message {
	out := slices.Clone(h)
	slices.Reverse(out)
	return out
}

func (h History) Contains(id int64) *Message {
	for i := range h {
		if h[i].ID == id {
			return &h[i]
		}
	}
	return nil
}

const (
	historyMin  = 12
	historyStep = 12
)

// HistoryLimit grows the window from 12 to 23 messages and then jumps back, so its start stays fixed
// for a while and the prompt prefix is served from the cache.
func HistoryLimit(totalMessages int) int {
	return historyMin + totalMessages%historyStep
}
