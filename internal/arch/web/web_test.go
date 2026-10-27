package web

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"dasyra/internal/app/tool"
)

var (
	ctx   = context.Background()
	quiet = slog.New(slog.NewTextHandler(io.Discard, nil))
)

func serve(t *testing.T, h http.HandlerFunc) string {
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestIsPublic(t *testing.T) {
	for addr, want := range map[string]bool{
		"8.8.8.8": true, "2a00:1450:4001::200e": true,
		"127.0.0.1": false, "10.1.2.3": false, "172.20.0.5": false, "192.168.1.1": false, "169.254.169.254": false,
		"100.64.12.34": false, "0.1.2.3": false, "250.1.1.1": false, "::1": false, "fd00::1": false, "::ffff:127.0.0.1": false,
	} {
		if got := isPublic(netip.MustParseAddr(addr)); got != want {
			t.Errorf("%s: got %v", addr, got)
		}
	}
}

func TestCheckURL(t *testing.T) {
	for _, raw := range []string{"ftp://example.com", "file:///etc/passwd", "http://", "example.com"} {
		if _, err := checkURL(raw); !errors.Is(err, tool.ErrNotPublic) {
			t.Errorf("%s: got %v", raw, err)
		}
	}
}

func TestPublicOnlyClientRefusesLocalServers(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("secret")) })
	_, err := NewPages(PDF{}, false).Read(ctx, url)
	if !errors.Is(err, tool.ErrNotPublic) {
		t.Fatalf("got %v", err)
	}
	if _, err := NewJSON(false).Query(ctx, url, "."); !errors.Is(err, tool.ErrNotPublic) {
		t.Fatalf("json got %v", err)
	}
}

func TestMinimize(t *testing.T) {
	long := strings.Repeat("Важный текст статьи. ", 10)
	cases := map[string]string{
		`<html><head><title>x</title></head><body><nav>Menu</nav><p>Hello &amp; welcome</p><script>var a=1</script></body></html>`: "Hello & welcome",
		`<div><div>short</div><p>` + long + `</p></div>`:                         strings.TrimSpace(long),
		`<div>tiny</div><!-- comment --><footer>f</footer><p>  a  <b>b</b> </p>`: "a b",
		`<div><div>x</div></div>`:                                                "",
	}
	for page, want := range cases {
		if got := minimize(page); got != want {
			t.Errorf("minimize(%.40q) = %q, want %q", page, got, want)
		}
	}
}

func TestPagesReadsHTMLAndSkipsBinaries(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/img":
			w.Header().Set("Content-Type", "image/png")
			w.Write([]byte{0x89, 'P', 'N', 'G'})
		case "/missing":
			http.NotFound(w, r)
		case "/redirect":
			http.Redirect(w, r, "/", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte("<body><p>Menu prices</p></body>"))
		}
	})
	pages := NewPages(PDF{}, true)
	if p, err := pages.Read(ctx, url+"/redirect"); err != nil || p.Text != "Menu prices" || p.Document {
		t.Fatalf("html: %+v %v", p, err)
	}
	if p, err := pages.Read(ctx, url+"/img"); err == nil || p.Text != "" {
		t.Fatalf("binary: %+v %v", p, err)
	}
	if p, _ := pages.Read(ctx, url+"/missing"); p.Text != "" {
		t.Fatalf("404 must be empty: %+v", p)
	}
}

type fakePDF string

func (f fakePDF) Extract(context.Context, []byte) string { return string(f) }

func TestPagesSendsPDFsToTheExtractor(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("\n%PDF-1.7 binary")) })
	p, err := NewPages(fakePDF("price list"), true).Read(ctx, url)
	if err != nil || p.Text != "price list" || !p.Document {
		t.Fatalf("got %+v %v", p, err)
	}
}

func TestJSONQuery(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/moved":
			http.Redirect(w, r, "/", http.StatusMovedPermanently)
		case "/html":
			w.Write([]byte("<html>"))
		default:
			w.Write([]byte(`[{"name":"Axe","win":0.53},{"name":"Lina","win":0.49}]`))
		}
	})
	api := NewJSON(true)
	out, err := api.Query(ctx, url, `[.[] | select(.win > 0.5) | .name]`)
	if err != nil || out != `["Axe"]` {
		t.Fatalf("got %q %v", out, err)
	}
	if out, _ := api.Query(ctx, url, `.[] | .name`); out != "\"Axe\"\n\"Lina\"" {
		t.Fatalf("one value per line: %q", out)
	}
	if _, err := api.Query(ctx, url+"/moved", "."); err == nil || err.Error() != "HTTP 301 redirect, use the final URL" {
		t.Fatalf("redirect: %v", err)
	}
	var filterErr *tool.FilterError
	for _, c := range []struct{ path, filter string }{{"/", ".[[["}, {"/", ".[0].name | tonumber"}, {"/html", "."}} {
		if _, err := api.Query(ctx, url+c.path, c.filter); !errors.As(err, &filterErr) {
			t.Errorf("%s %s: got %v", c.path, c.filter, err)
		}
	}
}

func TestNormalize(t *testing.T) {
	got := normalize([]map[string]any{
		{"title": " Menu ", "url": "https://mcd.pl", "content": "Big\n\n  Mac"},
		{"title": "local", "url": "/relative"},
		{"title": "no snippet", "url": "http://x.pl"},
	}, "content")
	want := []tool.SearchResult{{Title: "Menu", URL: "https://mcd.pl", Snippet: "Big Mac"}, {Title: "no snippet", URL: "http://x.pl"}}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %+v", got)
	}
}

func TestSearchFallsBackWhenSearxngIsEmptyOrDown(t *testing.T) {
	empty := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("format") != "json" {
			t.Error("format=json expected")
		}
		w.Write([]byte(`{"results":[]}`))
	})
	s := NewSearch(empty, "", quiet)
	s.Throttle.Interval = 0
	s.Fallback = func(context.Context, string) ([]tool.SearchResult, error) {
		return []tool.SearchResult{{Title: "ddg", URL: "https://ddg.gg"}}, nil
	}
	if got, err := s.Search(ctx, "q"); err != nil || len(got) != 1 || got[0].Title != "ddg" {
		t.Fatalf("fallback: %+v %v", got, err)
	}

	s.URL = ""
	s.Fallback = func(context.Context, string) ([]tool.SearchResult, error) { return nil, errors.New("rate limited") }
	if _, err := s.Search(ctx, "q"); err == nil {
		t.Fatal("both engines failed: error expected")
	}
}

func TestSearchTriesBraveBeforeDdgr(t *testing.T) {
	down := serve(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) })
	brave := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "key" || r.URL.Query().Get("q") != "market" {
			t.Errorf("request %v %v", r.Header, r.URL)
		}
		w.Write([]byte(`{"web":{"results":[{"title":"Farmers &amp; market","url":"https://city.example/m","description":"Opens on <strong>Saturday</strong>"}]}}`))
	})
	s := NewSearch(down, "key", quiet)
	s.BraveURL = brave
	s.Throttle.Interval = 0
	s.Fallback = func(context.Context, string) ([]tool.SearchResult, error) {
		t.Fatal("no ddgr expected")
		return nil, nil
	}
	got, err := s.Search(ctx, "market")
	if err != nil || len(got) != 1 || got[0].Title != "Farmers & market" || got[0].Snippet != "Opens on Saturday" {
		t.Fatalf("got %+v %v", got, err)
	}

	s.BraveURL = down
	s.Fallback = func(context.Context, string) ([]tool.SearchResult, error) {
		return []tool.SearchResult{{Title: "ddg", URL: "https://ddg.gg"}}, nil
	}
	if got, err := s.Search(ctx, "market"); err != nil || len(got) != 1 || got[0].Title != "ddg" {
		t.Fatalf("ddgr after Brave: %+v %v", got, err)
	}
}

func TestSearchUsesSearxngResults(t *testing.T) {
	url := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[{"title":"Farmers market","url":"https://city.example/market","content":"Opens  on Saturday"}]}`))
	})
	s := NewSearch(url, "", quiet)
	s.Fallback = func(context.Context, string) ([]tool.SearchResult, error) {
		t.Fatal("no fallback expected")
		return nil, nil
	}
	got, err := s.Search(ctx, "farmers market")
	if err != nil || len(got) != 1 || got[0].Snippet != "Opens on Saturday" {
		t.Fatalf("got %+v %v", got, err)
	}
}

func TestThrottleSpacesCalls(t *testing.T) {
	th := &Throttle{Interval: 30 * time.Millisecond}
	start := time.Now()
	for range 3 {
		th.Wait(ctx)
	}
	if elapsed := time.Since(start); elapsed < 60*time.Millisecond {
		t.Fatalf("three calls took only %v", elapsed)
	}
}

func TestCleanPDFText(t *testing.T) {
	got := cleanPDFText("  Pizza      12 zł\fPasta\t\t\t9 zł\n\n\n\nEnd ")
	if want := "Pizza  12 zł\nPasta  9 zł\n\nEnd"; got != want {
		t.Fatalf("got %q", got)
	}
}
