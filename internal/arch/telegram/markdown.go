package telegram

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// SendMarkdown shows the usual Markdown of a chat model (headings, bold, code blocks, links, tables) as Telegram
// HTML. A chunk Telegram refuses as HTML goes as plain text, so the answer always arrives.
func (c *Client) SendMarkdown(ctx context.Context, chatID int64, text string) error {
	for _, chunk := range fencedChunks(text, maxMarkdown) {
		err := c.call(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": markdownHTML(chunk), "parse_mode": "HTML"}, nil)
		if err != nil {
			err = c.Send(ctx, chatID, chunk)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// maxMarkdown leaves room for the tags and entities the text grows by.
const maxMarkdown = 3000

var fence = regexp.MustCompile("^\\s*```")

// fencedChunks cuts text like chunks, and a code block cut in two is closed and reopened, so both halves stay code.
func fencedChunks(text string, limit int) []string {
	parts := chunks(text, limit)
	open := ""
	for i, part := range parts {
		if open != "" {
			part = open + "\n" + part
		}
		open = ""
		for _, line := range strings.Split(part, "\n") {
			if fence.MatchString(line) {
				if open == "" {
					open = strings.TrimSpace(line)
				} else {
					open = ""
				}
			}
		}
		if open != "" {
			part += "\n```"
		}
		parts[i] = part
	}
	return parts
}

var (
	heading    = regexp.MustCompile(`^#{1,6}\s+(.+?)\s*#*$`)
	bullet     = regexp.MustCompile(`^(\s*)[-*+]\s+(.*)$`)
	rule       = regexp.MustCompile(`^\s*([-*_])(\s*[-*_]){2,}\s*$`)
	quote      = regexp.MustCompile(`^\s*>\s?(.*)$`)
	tableRow   = regexp.MustCompile(`^\s*\|.*\|\s*$`)
	tableRule  = regexp.MustCompile(`^\s*\|?[\s:|-]+\|?\s*$`)
	inlineCode = regexp.MustCompile("`([^`\n]+)`")
	link       = regexp.MustCompile(`\[([^\]\n]+)\]\((https?://[^\s)]+)\)`)
	bold       = regexp.MustCompile(`\*\*([^*\n]+?)\*\*|__([^_\n]+?)__`)
	italic     = regexp.MustCompile(`(^|[^\w*])\*([^*\s][^*\n]*?)\*`)
	strike     = regexp.MustCompile(`~~([^~\n]+?)~~`)
	codeSpan   = regexp.MustCompile("\x00\\d+\x00")
)

// markdownHTML turns Markdown into the few tags Telegram knows; everything else is escaped text.
func markdownHTML(md string) string {
	var out []string
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		switch {
		case fence.MatchString(line):
			lang := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "`"))
			var code []string
			for i++; i < len(lines) && !fence.MatchString(lines[i]); i++ {
				code = append(code, lines[i])
			}
			open := "<pre>"
			if lang != "" {
				open = `<pre><code class="language-` + html.EscapeString(lang) + `">`
			}
			close := "</pre>"
			if lang != "" {
				close = "</code></pre>"
			}
			out = append(out, open+html.EscapeString(strings.Join(code, "\n"))+close)
		case tableRow.MatchString(line):
			// Telegram has no tables, monospace keeps the columns
			var rows []string
			for ; i < len(lines) && tableRow.MatchString(lines[i]); i++ {
				if !tableRule.MatchString(lines[i]) {
					rows = append(rows, strings.TrimSpace(lines[i]))
				}
			}
			i--
			out = append(out, "<pre>"+html.EscapeString(strings.Join(rows, "\n"))+"</pre>")
		case quote.MatchString(line):
			var quoted []string
			for ; i < len(lines) && quote.MatchString(lines[i]); i++ {
				quoted = append(quoted, inline(quote.FindStringSubmatch(lines[i])[1]))
			}
			i--
			out = append(out, "<blockquote>"+strings.Join(quoted, "\n")+"</blockquote>")
		case rule.MatchString(line):
			out = append(out, "──────")
		case heading.MatchString(line):
			out = append(out, "<b>"+inline(heading.FindStringSubmatch(line)[1])+"</b>")
		case bullet.MatchString(line):
			m := bullet.FindStringSubmatch(line)
			out = append(out, m[1]+"• "+inline(m[2]))
		default:
			out = append(out, inline(line))
		}
	}
	return strings.Join(out, "\n")
}

// inline formats one line. Code spans keep their text as it is and may sit inside bold, so they are put aside
// while the rest is formatted.
func inline(text string) string {
	var spans []string
	text = inlineCode.ReplaceAllStringFunc(text, func(m string) string {
		spans = append(spans, "<code>"+html.EscapeString(strings.Trim(m, "`"))+"</code>")
		return fmt.Sprintf("\x00%d\x00", len(spans)-1)
	})
	text = html.EscapeString(text)
	text = link.ReplaceAllString(text, `<a href="$2">$1</a>`)
	text = bold.ReplaceAllString(text, "<b>$1$2</b>")
	text = italic.ReplaceAllString(text, "$1<i>$2</i>")
	text = strike.ReplaceAllString(text, "<s>$1</s>")
	return codeSpan.ReplaceAllStringFunc(text, func(m string) string {
		i, _ := strconv.Atoi(strings.Trim(m, "\x00"))
		return spans[i]
	})
}
