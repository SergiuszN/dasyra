// Package telegram is the Bot API: sending, reactions, files, webhook and long polling.
package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	apiURL         = "https://api.telegram.org"
	requestTimeout = 30 * time.Second
	maxFileBytes   = 20 << 20 // bots can download files up to 20 MB
)

type Client struct {
	Token   string
	Log     *slog.Logger
	BaseURL string // empty for Telegram itself
	HTTP    *http.Client
}

// Send writes plain text: everything MarkdownV2 would read as markup is escaped. A text over Telegram's
// limit goes as several messages, a long recipe or note was refused whole before.
func (c *Client) Send(ctx context.Context, chatID int64, text string) error {
	for _, chunk := range chunks(text, maxMessage) {
		if err := c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": escapeMarkdown(chunk), "parse_mode": "MarkdownV2"}, nil); err != nil {
			return err
		}
	}
	return nil
}

// maxMessage stays under Telegram's 4096 characters of a message.
const maxMessage = 4000

// chunks cuts text into pieces of at most limit characters, at the last line break when there is one.
func chunks(text string, limit int) []string {
	var out []string
	runes := []rune(text)
	for len(runes) > limit {
		cut, next := limit, limit
		for i := limit; i > limit/2; i-- {
			if runes[i] == '\n' {
				cut, next = i, i+1
				break
			}
		}
		out = append(out, strings.TrimSpace(string(runes[:cut])))
		runes = runes[next:]
	}
	return append(out, strings.TrimSpace(string(runes)))
}

func (c *Client) React(ctx context.Context, chatID, messageID int64, emoji string) error {
	return c.call(ctx, "setMessageReaction", map[string]any{
		"chat_id": chatID, "message_id": messageID, "reaction": []map[string]string{{"type": "emoji", "emoji": emoji}},
	}, nil)
}

func (c *Client) Typing(ctx context.Context, chatID int64) {
	if err := c.call(ctx, "sendChatAction", map[string]any{"chat_id": chatID, "action": "typing"}, nil); err != nil {
		c.Log.Debug("Typing failed", "error", err)
	}
}

// Download fetches a file the user sent.
func (c *Client) Download(ctx context.Context, fileID string) ([]byte, error) {
	var file struct {
		FilePath string `json:"file_path"`
	}
	if err := c.call(ctx, "getFile", map[string]any{"file_id": fileID}, &file); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base()+"/file/bot"+c.Token+"/"+file.FilePath, nil)
	if err != nil {
		return nil, err
	}
	res, err := c.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: HTTP %d", file.FilePath, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxFileBytes))
}

// Username is the bot's @name, for invite links.
func (c *Client) Username(ctx context.Context) (string, error) {
	var me struct {
		Username string `json:"username"`
	}
	err := c.call(ctx, "getMe", map[string]any{}, &me)
	return me.Username, err
}

// Updates long-polls for new updates, for running without a webhook.
func (c *Client) Updates(ctx context.Context, offset int64) ([]Update, error) {
	var updates []Update
	err := c.call(ctx, "getUpdates", map[string]any{"offset": offset, "timeout": 25}, &updates)
	return updates, err
}

func (c *Client) call(ctx context.Context, method string, params map[string]any, result any) error {
	body, _ := json.Marshal(params)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout+time.Duration(asInt(params["timeout"]))*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base()+"/bot"+c.Token+"/"+method, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	res, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	defer res.Body.Close()
	var reply struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.NewDecoder(res.Body).Decode(&reply); err != nil {
		return fmt.Errorf("telegram %s: %w", method, err)
	}
	if !reply.OK {
		c.Log.Error("Telegram API returned error", "method", method, "description", reply.Description)
		return errors.New("telegram " + method + ": " + reply.Description)
	}
	if result != nil {
		return json.Unmarshal(reply.Result, result)
	}
	return nil
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return c.BaseURL
	}
	return apiURL
}

func (c *Client) client() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return http.DefaultClient
}

func asInt(v any) int {
	n, _ := v.(int)
	return n
}

var markdownSpecial = regexp.MustCompile("([_*\\[\\]()~`>#+=|{}.!-])")

func escapeMarkdown(text string) string {
	return markdownSpecial.ReplaceAllString(strings.ReplaceAll(text, `\`, `\\`), `\$1`)
}
