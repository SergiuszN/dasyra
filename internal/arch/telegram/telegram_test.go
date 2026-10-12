package telegram

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dasyra/internal/app"
)

var discard = slog.New(slog.NewTextHandler(io.Discard, nil))

func parse(t *testing.T, raw string) app.Incoming {
	t.Helper()
	var m Message
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatal(err)
	}
	return m.Incoming()
}

func TestPhotoPicksTheLargestSizeUpTo1280(t *testing.T) {
	in := parse(t, `{"message_id":5,"chat":{"id":9,"type":"private"},"caption":"look",
		"photo":[{"file_id":"s","width":90,"height":60},{"file_id":"m","width":1280,"height":853},{"file_id":"l","width":2560,"height":1706}]}`)
	if in.ImageFileID != "m" || in.Text != "look" || !in.Private {
		t.Fatalf("%+v", in)
	}
}

func TestDocumentsAndAudio(t *testing.T) {
	pdf := parse(t, `{"message_id":1,"chat":{"id":9,"type":"private"},"document":{"file_id":"d","mime_type":"application/pdf"}}`)
	word := parse(t, `{"message_id":1,"chat":{"id":9,"type":"private"},"document":{"file_id":"w","mime_type":"application/msword"}}`)
	image := parse(t, `{"message_id":1,"chat":{"id":9,"type":"private"},"document":{"file_id":"i","mime_type":"image/png"}}`)
	voice := parse(t, `{"message_id":1,"chat":{"id":9,"type":"group"},"voice":{"file_id":"v"}}`)
	if pdf.DocumentFileID != "d" || pdf.DocumentName != "document" || word.DocumentFileID != "" || image.ImageFileID != "i" {
		t.Fatalf("pdf %+v word %+v image %+v", pdf, word, image)
	}
	if voice.AudioFileID != "v" || voice.AudioFileName != "voice.ogg" || voice.Private {
		t.Fatalf("voice %+v", voice)
	}
}

func TestEscapeMarkdown(t *testing.T) {
	if got := escapeMarkdown(`1.5 * (a_b) - c\d!`); got != `1\.5 \* \(a\_b\) \- c\\d\!` {
		t.Fatalf("got %s", got)
	}
}

type inbox struct{ got []app.Incoming }

func (i *inbox) Receive(_ context.Context, m app.Incoming) error {
	i.got = append(i.got, m)
	return nil
}

func TestWebhookChecksTheSecret(t *testing.T) {
	in := &inbox{}
	h := &Webhook{Secret: "s3cret", Inbox: in, Log: discard}
	body := `{"update_id":1,"message":{"message_id":2,"chat":{"id":3,"type":"private"},"text":"hi"}}`

	rejected := httptest.NewRecorder()
	h.ServeHTTP(rejected, httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body)))
	accepted := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/telegram/webhook", strings.NewReader(body))
	req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "s3cret")
	h.ServeHTTP(accepted, req)

	if rejected.Code != http.StatusForbidden || accepted.Code != http.StatusOK || len(in.got) != 1 || in.got[0].Text != "hi" {
		t.Fatalf("rejected %d accepted %d got %+v", rejected.Code, accepted.Code, in.got)
	}
}

func TestClientSendsEscapedTextAndDownloads(t *testing.T) {
	var sent map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/sendMessage"):
			_ = json.NewDecoder(r.Body).Decode(&sent)
			_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
		case strings.HasSuffix(r.URL.Path, "/getFile"):
			_, _ = w.Write([]byte(`{"ok":true,"result":{"file_path":"photos/a.jpg"}}`))
		case r.URL.Path == "/file/botT/photos/a.jpg":
			_, _ = w.Write([]byte("JPEG"))
		case strings.HasSuffix(r.URL.Path, "/setMessageReaction"):
			_, _ = w.Write([]byte(`{"ok":false,"description":"REACTION_INVALID"}`))
		}
	}))
	defer server.Close()
	c := &Client{Token: "T", Log: discard, BaseURL: server.URL}
	ctx := context.Background()

	if err := c.Send(ctx, 9, "Привет!"); err != nil || sent["text"] != `Привет\!` || sent["parse_mode"] != "MarkdownV2" {
		t.Fatalf("sent %v %v", sent, err)
	}
	if data, err := c.Download(ctx, "f"); err != nil || string(data) != "JPEG" {
		t.Fatalf("download %q %v", data, err)
	}
	if err := c.React(ctx, 9, 1, "🦄"); err == nil {
		t.Fatal("a refused reaction must fail")
	}
}

func TestChunksCutLongTextAtLineBreaks(t *testing.T) {
	if got := chunks("short", 10); len(got) != 1 || got[0] != "short" {
		t.Fatalf("got %q", got)
	}
	got := chunks("aaaa\nbbbb\ncccccccccccc", 10)
	if len(got) != 3 || got[0] != "aaaa\nbbbb" || got[1] != "cccccccccc" || got[2] != "cc" {
		t.Fatalf("got %q", got)
	}
}
