package tool

import (
	"context"
	"strings"
	"unicode/utf8"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

const ReadDocumentName = "read_document"

// ReadDocument reads a PDF or text file sent in the chat the way web_fetch reads pages: a cheap pass answers the question.
type ReadDocument struct {
	Messages dom.Messages
	Files    Files
	PDF      PDFText
	Reader   TextReader
}

func (ReadDocument) Spec() llm.Tool {
	return Spec(ReadDocumentName, `Read a PDF or text file the user sent in this chat (shown as [file "name", read_document id N]) and answer a question about it. Ask again with another question to look up something else in the same file.`,
		Int("id", "The read_document id shown next to the file").Required(),
		Str("question", "What to find in the file, leave empty for a summary").Required(),
	)
}

func (t ReadDocument) Run(ctx context.Context, s *Session, a Args) (string, error) {
	fileID := ""
	if id := a.Int("id"); id > 0 {
		var err error
		if fileID, err = t.Messages.DocumentFileID(ctx, s.User.ID, id); err != nil {
			return "", err
		}
	}
	if fileID == "" {
		return "Error: no file with this id in the chat.", nil
	}

	data, err := t.Files.Download(ctx, fileID)
	if err != nil {
		return "Error: could not download the file (Telegram lets bots download files up to 20 MB).", nil
	}

	text := t.text(ctx, data)
	switch {
	case text == "" && isPDF(data):
		return "Error: this PDF has no text layer (a scan or images), it can't be read.", nil
	case text == "":
		return "Error: the file is not readable text.", nil
	}
	return t.Reader.Answer(ctx, text, a.String("question"), documentLimit, "document"), nil
}

// text is the PDF text layer or the file itself when it is text, "" otherwise: binaries would break the prompt.
func (t ReadDocument) text(ctx context.Context, data []byte) string {
	if isPDF(data) {
		return t.PDF.Extract(ctx, data)
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return ""
	}
	return strings.TrimSpace(string(data))
}
