package llmapi

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"strings"
	"time"

	"dasyra/internal/app/llm"
)

const transcriptionTimeout = 60 * time.Second

// Transcribe turns a voice message into text once, on arrival: the transcript is stored as a normal message and
// the audio is not kept. Whisper takes Telegram's OGG/Opus as is.
func (c *Client) Transcribe(ctx context.Context, audio []byte, fileName string) (string, error) {
	return c.transcribe(ctx, llm.Transcription.ID, audio, fileName)
}

func (c *Client) transcribe(ctx context.Context, model string, audio []byte, fileName string) (string, error) {
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("model", model)
	file, err := form.CreateFormFile("file", fileName)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(audio); err != nil {
		return "", err
	}
	if err := form.Close(); err != nil {
		return "", err
	}

	ctx, cancel := context.WithTimeout(ctx, transcriptionTimeout)
	defer cancel()
	var resp struct {
		Text  *string `json:"text"`
		Usage *usage  `json:"usage"`
	}
	if err := c.post(ctx, c.url("/audio/transcriptions"), form.FormDataContentType(), &body, &resp); err != nil {
		return "", err
	}
	if resp.Usage != nil {
		c.record(ctx, model, *resp.Usage)
	}
	if resp.Text == nil {
		return "", errors.New("no transcript in the response")
	}
	c.Log.Info("Voice transcribed", "chars", len([]rune(*resp.Text)))
	return strings.TrimSpace(*resp.Text), nil
}
