// Package app holds the use cases: replying, check-ins, reminders, nightly reflection and incoming messages.
package app

import (
	"context"
	"sync"

	"dasyra/internal/app/tool"
)

// Messenger is the chat Dasyra talks in.
type Messenger interface {
	Send(ctx context.Context, chatID int64, text string) error
	React(ctx context.Context, chatID, messageID int64, emoji string) error
	Typing(ctx context.Context, chatID int64)
	tool.Files
}

// Transcriber turns a voice message into text.
type Transcriber interface {
	Transcribe(ctx context.Context, audio []byte, fileName string) (string, error)
}

// userLocks give each user one run at a time: a reply, a check-in and a reminder never overlap.
type userLocks struct{ m sync.Map }

func (l *userLocks) lock(userID int64) func() {
	mu, _ := l.m.LoadOrStore(userID, &sync.Mutex{})
	mu.(*sync.Mutex).Lock()
	return mu.(*sync.Mutex).Unlock
}
