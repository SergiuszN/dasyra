package telegram

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"dasyra/internal/app"
)

type Receiver interface {
	Receive(ctx context.Context, m app.Incoming) error
}

// Webhook takes updates from Telegram. It answers once the message is stored, the reply runs in the background:
// Telegram sends the next update only after this one is answered.
type Webhook struct {
	Secret string
	Inbox  Receiver
	Log    *slog.Logger
}

func (h *Webhook) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.Secret != "" && subtle.ConstantTimeCompare([]byte(h.Secret), []byte(r.Header.Get("X-Telegram-Bot-Api-Secret-Token"))) != 1 {
		h.Log.Info("Rejected Telegram webhook call because of invalid secret token.")
		writeJSON(w, http.StatusForbidden, false)
		return
	}

	var u Update
	if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
		h.Log.Error("Failed to parse Telegram webhook payload.", "error", err)
	} else if u.Message != nil {
		h.receive(r.Context(), u)
	}
	writeJSON(w, http.StatusOK, true)
}

func (h *Webhook) receive(ctx context.Context, u Update) {
	if err := h.Inbox.Receive(ctx, u.Message.Incoming()); err != nil {
		h.Log.Error("Failed to process Telegram webhook update.", "update_id", u.UpdateID, "error", err)
	}
}

func writeJSON(w http.ResponseWriter, status int, ok bool) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]bool{"ok": ok})
}

// Poll reads updates without a webhook, for local runs.
func Poll(ctx context.Context, c *Client, inbox Receiver, log *slog.Logger) {
	var offset int64
	for ctx.Err() == nil {
		updates, err := c.Updates(ctx, offset)
		if err != nil {
			log.Error("Polling failed", "error", err)
			time.Sleep(5 * time.Second)
			continue
		}
		for _, u := range updates {
			offset = u.UpdateID + 1
			if u.Message != nil {
				(&Webhook{Inbox: inbox, Log: log}).receive(ctx, u)
			}
		}
	}
}
