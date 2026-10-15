package telegram

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMarkdownBecomesTelegramHTML(t *testing.T) {
	md := strings.Join([]string{
		"## Ответ",
		"Это **важно**, *очень* и ~~нет~~, см. [доку](https://php.net/a?b=1&c=2) и `$x < 3`.",
		"- первый",
		"* второй",
		"```php",
		"if ($a && $b) { echo \"<p>\"; }",
		"",
		"return **$x**;",
		"```",
		"| A | B |",
		"|---|---|",
		"| 1 | 2 |",
		"> цитата",
		"---",
		"2 * 3 * 4",
		"**Сначала `__clone()`, потом** _a_b_",
	}, "\n")
	want := strings.Join([]string{
		"<b>Ответ</b>",
		`Это <b>важно</b>, <i>очень</i> и <s>нет</s>, см. <a href="https://php.net/a?b=1&amp;c=2">доку</a> и <code>$x &lt; 3</code>.`,
		"• первый",
		"• второй",
		`<pre><code class="language-php">if ($a &amp;&amp; $b) { echo &#34;&lt;p&gt;&#34;; }`,
		"",
		"return **$x**;</code></pre>",
		"<pre>| A | B |",
		"| 1 | 2 |</pre>",
		"<blockquote>цитата</blockquote>",
		"──────",
		"2 * 3 * 4",
		"<b>Сначала <code>__clone()</code>, потом</b> _a_b_",
	}, "\n")
	if got := markdownHTML(md); got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestCodeBlockCutInTwoStaysCode(t *testing.T) {
	code := "```go\n" + strings.Repeat("x := 1\n", 30) + "```"
	parts := fencedChunks(code, 100)
	if len(parts) < 2 {
		t.Fatalf("got %q", parts)
	}
	for _, p := range parts {
		if !strings.HasPrefix(p, "```go") || !strings.HasSuffix(p, "```") {
			t.Fatalf("part is not a whole block: %q", p)
		}
	}
}

func TestRefusedHTMLGoesAsPlainText(t *testing.T) {
	var sent []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		sent = append(sent, params)
		if params["parse_mode"] == "HTML" {
			_, _ = w.Write([]byte(`{"ok":false,"description":"can't parse entities"}`))
			return
		}
		_, _ = w.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer server.Close()
	c := &Client{Token: "T", Log: discard, BaseURL: server.URL}

	if err := c.SendMarkdown(context.Background(), 9, "**Да!**"); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 2 || sent[0]["text"] != "<b>Да!</b>" || sent[1]["text"] != `\*\*Да\!\*\*` {
		t.Fatalf("sent %v", sent)
	}
}
