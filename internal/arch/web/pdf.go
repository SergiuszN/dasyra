package web

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const pdfTimeout = 30 * time.Second

var (
	layoutSpaces = regexp.MustCompile(`[ \t]{3,}`)
	blankLines   = regexp.MustCompile(`\n{3,}`)
)

// PDF extracts the text layer with poppler's pdftotext.
type PDF struct {
	Log *slog.Logger
}

// Extract returns "" when nothing readable comes out, e.g. a scan without a text layer.
func (p PDF) Extract(ctx context.Context, pdf []byte) string {
	dir, err := os.MkdirTemp("", "pdf")
	if err != nil {
		return ""
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in.pdf"), filepath.Join(dir, "out.txt")
	if err := os.WriteFile(in, pdf, 0o600); err != nil {
		return ""
	}

	// A broken or huge PDF must not hang the reply
	ctx, cancel := context.WithTimeout(ctx, pdfTimeout)
	defer cancel()
	// -layout keeps table columns side by side, so prices and their labels stay on one line
	if err := exec.CommandContext(ctx, "pdftotext", "-layout", "-enc", "UTF-8", in, out).Run(); err != nil && p.Log != nil {
		p.Log.Error("pdftotext failed", "error", err)
	}
	text, _ := os.ReadFile(out)
	return cleanPDFText(string(text))
}

// cleanPDFText drops what only costs tokens: page breaks and runs of layout spaces.
func cleanPDFText(text string) string {
	text = strings.ReplaceAll(strings.ToValidUTF8(text, "?"), "\f", "\n")
	text = layoutSpaces.ReplaceAllString(text, "  ")
	return strings.TrimSpace(blankLines.ReplaceAllString(text, "\n\n"))
}
