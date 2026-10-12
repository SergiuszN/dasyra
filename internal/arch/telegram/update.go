package telegram

import (
	"slices"
	"strings"

	"dasyra/internal/app"
)

type Update struct {
	UpdateID int64    `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	MessageID int64 `json:"message_id"`
	From      struct {
		FirstName string `json:"first_name"`
	} `json:"from"`
	Chat struct {
		ID   int64  `json:"id"`
		Type string `json:"type"`
	} `json:"chat"`
	Text      string  `json:"text"`
	Caption   string  `json:"caption"`
	Photo     []photo `json:"photo"`
	Document  *file   `json:"document"`
	Voice     *file   `json:"voice"`
	Audio     *file   `json:"audio"`
	VideoNote *file   `json:"video_note"`
}

type photo struct {
	FileID        string `json:"file_id"`
	Width, Height int
}

type file struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
}

var imageTypes = []string{"image/jpeg", "image/png", "image/webp", "image/gif"}

// Incoming turns a Telegram message into what the inbox needs.
func (m *Message) Incoming() app.Incoming {
	in := app.Incoming{
		ChatID: m.Chat.ID, MessageID: m.MessageID, Private: m.Chat.Type == "private",
		FirstName: m.From.FirstName, Text: m.Text, ImageFileID: m.image(),
	}
	if in.Text == "" {
		in.Text = m.Caption
	}
	if d := m.Document; in.ImageFileID == "" && d != nil && (d.MimeType == "application/pdf" || strings.HasPrefix(d.MimeType, "text/")) {
		in.DocumentFileID, in.DocumentName = d.FileID, orDefault(d.FileName, "document")
	}
	for _, a := range []struct {
		f    *file
		name string
	}{{m.Voice, "voice.ogg"}, {m.Audio, "audio.mp3"}, {m.VideoNote, "video.mp4"}} {
		if a.f != nil && a.f.FileID != "" {
			in.AudioFileID, in.AudioFileName = a.f.FileID, orDefault(a.f.FileName, a.name)
			break
		}
	}
	return in
}

// image picks the largest photo size up to ~1280px, enough for the model; image documents count too.
func (m *Message) image() string {
	if len(m.Photo) > 0 {
		best := m.Photo[0]
		for _, size := range m.Photo {
			if max(size.Width, size.Height) <= 1280 {
				best = size
			}
		}
		return best.FileID
	}
	if m.Document != nil && slices.Contains(imageTypes, m.Document.MimeType) {
		return m.Document.FileID
	}
	return ""
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
