package httpd

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"dasyra/internal/dom"
)

func checkCookbook(_ context.Context, c dom.Cookbook) (string, error) {
	if c.Token != "mealie-token-good" {
		return "", errors.New("the cookbook refused the token (HTTP 401)")
	}
	return "user Alex, household Family", nil
}

func TestCookbookIsConnectedPerUserWithoutShowingTheToken(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/users/1")
	if !strings.Contains(page, "Not connected") {
		t.Fatal("no cookbook yet")
	}
	token := csrf(t, page)

	status, body := post(t, c, s.URL+"/panel/users/1/cookbook", url.Values{"csrf": {token}, "url": {"mealie:9000"}, "token": {"x"}})
	if status != http.StatusBadRequest || !strings.Contains(body, "must start with http") || !strings.Contains(body, `value="mealie:9000"`) {
		t.Fatalf("bad address: %d", status)
	}
	_, body = post(t, c, s.URL+"/panel/users/1/cookbook", url.Values{"csrf": {token}, "url": {"http://mealie:9000/"}, "public_url": {"https://recipes.example"}, "token": {"mealie-token-good"}})
	if !strings.Contains(body, "Cookbook saved") || !strings.Contains(body, "••••good") || strings.Contains(body, "mealie-token-good") || !strings.Contains(body, `value="http://mealie:9000"`) {
		t.Fatalf("saved: %s", body)
	}

	// A blank token keeps the saved one, the check gets it too
	if _, body = post(t, c, s.URL+"/panel/users/1/cookbook/check", url.Values{"csrf": {token}, "url": {"http://mealie:9000"}}); !strings.Contains(body, "The cookbook answers: user Alex, household Family.") {
		t.Fatalf("check: %s", body)
	}
	if _, body = post(t, c, s.URL+"/panel/users/1/cookbook/check", url.Values{"csrf": {token}, "url": {"http://mealie:9000"}, "token": {"bad"}}); !strings.Contains(body, "did not answer: the cookbook refused") {
		t.Fatalf("check with a wrong token: %s", body)
	}
	if status, _ := post(t, c, s.URL+"/panel/users/1/cookbook/delete", url.Values{"csrf": {"forged"}}); status != http.StatusForbidden {
		t.Fatalf("forged csrf: %d", status)
	}
	if _, body = post(t, c, s.URL+"/panel/users/1/cookbook/delete", url.Values{"csrf": {token}}); !strings.Contains(body, "Cookbook disconnected") || !strings.Contains(body, "Not connected") {
		t.Fatalf("delete: %s", body)
	}
	if status, _ := post(t, c, s.URL+"/panel/users/7/cookbook", url.Values{"csrf": {token}, "url": {"http://m"}, "token": {"t"}}); status != http.StatusNotFound {
		t.Fatalf("unknown user: %d", status)
	}
}
