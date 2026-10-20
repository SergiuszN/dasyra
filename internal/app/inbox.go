package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/app/tool"
	"dasyra/internal/dom"
)

const (
	// Wait a bit so a burst of messages or a photo album gets a single answer
	debounce        = 2 * time.Second
	redeliveryGuard = 24 * time.Hour
	inviteOnly      = "Dasyra is invite-only for now."
	noTranscript    = "(could not be transcribed, ask to resend it or write it as text)"
)

// Incoming is a message from the chat, already parsed by the transport.
type Incoming struct {
	ChatID, MessageID int64
	Private           bool
	FirstName         string
	Text              string
	ImageFileID       string
	DocumentFileID    string // PDFs and text files, read on demand by read_document
	DocumentName      string
	AudioFileID       string // voice messages, audio files and video notes, transcribed on arrival
	AudioFileName     string
}

func (m Incoming) empty() bool {
	return strings.TrimSpace(m.Text) == "" && m.ImageFileID == "" && m.DocumentFileID == "" && m.AudioFileID == ""
}

// Inbox stores incoming messages and schedules the reply. Receive returns once the message is stored,
// so the transport can release Telegram: it sends the next update only after this one is answered.
type Inbox struct {
	Users    dom.Users
	Invites  dom.Invites
	Messages dom.Messages
	Dasyra   *Dasyra
	Chat     Messenger
	Voice    Transcriber
	IDs      tool.IDs
	Log      *slog.Logger
	Debounce time.Duration
	Timezone string // of new users until they name theirs, the server zone
	Work     *Background
}

func (in *Inbox) Receive(ctx context.Context, m Incoming) error {
	if !m.Private {
		return nil
	}
	u, err := in.Users.ByChat(ctx, m.ChatID)
	if err != nil {
		return err
	}
	if u == nil {
		return in.stranger(ctx, m)
	}
	if u.Disabled() {
		return in.Chat.Send(ctx, m.ChatID, inviteOnly)
	}
	if m.empty() || strings.HasPrefix(strings.TrimSpace(m.Text), "/start") {
		return nil
	}
	if seen, err := in.Messages.Seen(ctx, u.ID, m.MessageID, now().Add(-redeliveryGuard)); err != nil || seen {
		return err
	}

	stored := &dom.Message{ID: in.IDs.Next(), Role: dom.RoleUser, Content: in.content(ctx, u, m), TelegramID: m.MessageID,
		CreatedAt: now(), ImageFileID: m.ImageFileID, DocumentFileID: m.DocumentFileID}
	if err := in.Messages.Insert(ctx, u.ID, stored); err != nil {
		return err
	}

	in.Work.Go(func(ctx context.Context) {
		time.Sleep(in.Debounce)
		// A newer message came meanwhile: its own handler answers them all at once
		if newer, err := in.Messages.HasUserMessageAfter(ctx, u.ID, stored.ID); err != nil || newer {
			return
		}
		in.reply(ctx, u.ID)
	})
	return nil
}

// content is the message as Dasyra sees it, attachments marked.
func (in *Inbox) content(ctx context.Context, u *dom.User, m Incoming) string {
	text := strings.TrimSpace(m.Text)
	switch {
	case m.ImageFileID != "":
		return strings.TrimSpace("[photo] " + text)
	case m.DocumentFileID != "":
		// The id is the handle read_document takes, the file itself is downloaded only when she reads it
		return strings.TrimSpace(fmt.Sprintf(`[file "%s", read_document id %d] %s`, m.DocumentName, m.MessageID, text))
	case m.AudioFileID != "":
		// Transcribed before Telegram is released, so a text sent right after a voice message cannot overtake it
		return strings.TrimSpace("[voice] " + in.transcribe(llm.For(ctx, u.ID, llm.PurposeTranscription), m) + " " + text)
	}
	return text
}

func (in *Inbox) transcribe(ctx context.Context, m Incoming) string {
	audio, err := in.Chat.Download(ctx, m.AudioFileID)
	if err != nil {
		in.Log.Error("Failed to download audio", "error", err)
		return noTranscript
	}
	text, err := in.Voice.Transcribe(ctx, audio, m.AudioFileName)
	if err != nil || text == "" {
		in.Log.Error("Voice transcription failed", "error", err)
		return noTranscript
	}
	return text
}

// stranger registers a new user by an invite code from /start, anyone else is turned away.
func (in *Inbox) stranger(ctx context.Context, m Incoming) error {
	text := strings.TrimSpace(m.Text)
	code := strings.TrimSpace(strings.TrimPrefix(text, "/start"))
	available := false
	if strings.HasPrefix(text, "/start") && code != "" {
		var err error
		if available, err = in.Invites.Available(ctx, code); err != nil {
			return err
		}
	}
	if !available {
		in.Log.Info("Rejected message from unknown Telegram chat.", "chat_id", m.ChatID)
		return in.Chat.Send(ctx, m.ChatID, inviteOnly)
	}

	u := &dom.User{ID: in.IDs.Next(), Name: m.FirstName, ChatID: m.ChatID, Timezone: in.timezone(), CreatedAt: now()}
	if err := in.Users.Insert(ctx, u); err != nil {
		return err
	}
	if err := in.Invites.Use(ctx, code, u.ID); err != nil {
		return err
	}
	in.Log.Info("New user registered by invite.", "user_id", u.ID, "code", code)
	in.Work.Go(func(ctx context.Context) { in.reply(ctx, u.ID) })
	return nil
}

// reply reloads the user, the profile may have changed since the message came.
func (in *Inbox) reply(ctx context.Context, userID int64) {
	u, err := in.Users.Get(ctx, userID)
	if err != nil || u == nil {
		in.Log.Error("Failed to load the user for a reply", "user_id", userID, "error", err)
		return
	}
	in.Dasyra.Speak(ctx, u, Run{})
}

func (in *Inbox) timezone() string {
	if in.Timezone == "" {
		return dom.DefaultTimezone
	}
	return in.Timezone
}
