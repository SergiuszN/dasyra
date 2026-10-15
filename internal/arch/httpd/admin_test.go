package httpd

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

// panel starts the admin on a fresh database with one recorded AI call and returns a client with a cookie jar.
func panel(t *testing.T, login, password string) (*httptest.Server, *http.Client) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "test.sqlite"), time.Local)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := (sqlite.Users{DB: db}).Update(context.Background(), &dom.User{ID: dom.OwnerID, Name: "Alex", Timezone: "Europe/Warsaw"}); err != nil {
		t.Fatal(err)
	}
	_ = sqlite.UsageLog{DB: db}.Record(context.Background(), dom.Usage{UserID: 1, Purpose: "chat", Model: "anthropic/claude-haiku-5.5", PromptTokens: 12000, CachedTokens: 9000, CompletionTokens: 300, Cost: 0.0123})

	admin := &Admin{Report: sqlite.Report{DB: db}, Users: sqlite.Users{DB: db}, AI: sqlite.AISettings{DB: db}, Plans: sqlite.Settings{DB: db}, CheckAI: checkAI, SuggestPrices: suggestPrices,
		Cookbooks: sqlite.Cookbooks{DB: db}, CheckCookbook: checkCookbook,
		Login: login, Password: password,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)), pause: time.Millisecond}
	mux := http.NewServeMux()
	admin.Register(mux)
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	jar, _ := cookiejar.New(nil)
	return server, &http.Client{Jar: jar}
}

var csrfField = regexp.MustCompile(`name="csrf" value="([0-9a-f]+)"`)

func get(t *testing.T, c *http.Client, url string) (int, string) {
	t.Helper()
	resp, err := c.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func post(t *testing.T, c *http.Client, url string, form url.Values) (int, string) {
	t.Helper()
	resp, err := c.PostForm(url, form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func csrf(t *testing.T, page string) string {
	t.Helper()
	m := csrfField.FindStringSubmatch(page)
	if m == nil {
		t.Fatalf("no csrf in page: %s", page)
	}
	return m[1]
}

func logIn(t *testing.T, s *httptest.Server, c *http.Client) string {
	t.Helper()
	_, page := get(t, c, s.URL+"/login")
	status, body := post(t, c, s.URL+"/login", url.Values{"login": {"admin"}, "password": {"secret"}, "csrf": {csrf(t, page)}})
	if status != http.StatusOK || !strings.Contains(body, "AI spend") {
		t.Fatalf("login: %d %s", status, body)
	}
	return body
}

func TestClosedPanel(t *testing.T) {
	s, c := panel(t, "", "")
	if _, body := get(t, c, s.URL+"/panel"); !strings.Contains(body, "Login is closed") {
		t.Fatalf("closed panel must send to the closed login page: %s", body)
	}
}

func TestFailedLogin(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	_, page := get(t, c, s.URL+"/login")
	status, body := post(t, c, s.URL+"/login", url.Values{"login": {"admin"}, "password": {"wrong"}, "csrf": {csrf(t, page)}})
	if status != http.StatusUnauthorized || !strings.Contains(body, "Wrong login or password.") {
		t.Fatalf("%d %s", status, body)
	}
	if status, body = post(t, c, s.URL+"/login", url.Values{"login": {"admin"}, "password": {"secret"}, "csrf": {"stale"}}); status != http.StatusUnauthorized || !strings.Contains(body, "The form has expired") {
		t.Fatalf("stale csrf: %d %s", status, body)
	}
}

func TestDashboardAfterLogin(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	body := logIn(t, s, c)
	for _, want := range []string{"$0.0123", "12.0k", "75%", "Chat replies", "claude-haiku-5.5", "Alex"} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard has no %q", want)
		}
	}
	if status, body := get(t, c, s.URL+"/panel/users/1?period=7"); status != http.StatusOK || !strings.Contains(body, "time zone Europe/Warsaw") {
		t.Fatalf("user page: %d", status)
	}
	if status, body := get(t, c, s.URL+"/panel/users/0"); status != http.StatusOK || !strings.Contains(body, "No user") {
		t.Fatalf("nobody's bucket: %d %s", status, body)
	}
}

func TestLogoutNeedsCSRF(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	body := logIn(t, s, c)
	post(t, c, s.URL+"/logout", url.Values{"csrf": {"forged"}})
	if _, page := get(t, c, s.URL+"/panel"); !strings.Contains(page, "AI spend") {
		t.Fatal("a forged logout must not log out")
	}
	post(t, c, s.URL+"/logout", url.Values{"csrf": {csrf(t, body)}})
	if _, page := get(t, c, s.URL+"/panel"); strings.Contains(page, "AI spend") {
		t.Fatal("logout with the token must log out")
	}
}

func TestUnknownUserIsNotFound(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	if status, _ := get(t, c, s.URL+"/panel/users/42"); status != http.StatusNotFound {
		t.Fatalf("status %d", status)
	}
}

func TestFormatting(t *testing.T) {
	cases := map[string]string{
		money(12.345): "$12.35", money(0.0123): "$0.0123", money(0.00012): "$0.000120", money(0): "$0.0000",
		tokens(1_234_567): "1.23M", tokens(12_345): "12.3k", tokens(9_999): "9 999",
		percent(1, 3): "33%", percent(1, 0): "—", fixed(1234567.891, 2): "1 234 567.89", minutes(90): "1.5 min",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
