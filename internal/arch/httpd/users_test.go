package httpd

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var inviteCode = regexp.MustCompile(`value="/start ([0-9a-f]{12})"`)

func TestInvitesAndAccessOnTheUsersPage(t *testing.T) {
	s, c := panel(t, "admin", "secret")
	logIn(t, s, c)
	_, page := get(t, c, s.URL+"/panel/users")
	if !strings.Contains(page, "Alex") || !strings.Contains(page, "owner") {
		t.Fatalf("users page: %s", page)
	}
	token := csrf(t, page)

	if status, _ := post(t, c, s.URL+"/panel/invites", url.Values{"csrf": {"forged"}}); status != http.StatusForbidden {
		t.Fatalf("forged csrf: %d", status)
	}
	_, body := post(t, c, s.URL+"/panel/invites", url.Values{"csrf": {token}, "note": {"Anna"}})
	m := inviteCode.FindStringSubmatch(body)
	if !strings.Contains(body, "Invite created") || m == nil || !strings.Contains(body, "Anna") {
		t.Fatalf("invite: %s", body)
	}
	if _, body = post(t, c, s.URL+"/panel/invites/revoke", url.Values{"csrf": {token}, "code": {m[1]}}); !strings.Contains(body, "Invite revoked") || strings.Contains(body, m[1]) {
		t.Fatalf("revoke: %s", body)
	}

	if status, body := post(t, c, s.URL+"/panel/users/1/access", url.Values{"csrf": {token}, "access": {"off"}}); status != http.StatusBadRequest || !strings.Contains(body, "owner cannot be turned off") {
		t.Fatalf("owner off: %d", status)
	}
	if _, body = post(t, c, s.URL+"/panel/users/1/access", url.Values{"csrf": {token}, "access": {"on"}}); !strings.Contains(body, "Access is back") {
		t.Fatalf("on: %s", body)
	}
	if status, _ := post(t, c, s.URL+"/panel/users/7/access", url.Values{"csrf": {token}, "access": {"off"}}); status != http.StatusNotFound {
		t.Fatalf("unknown user: %d", status)
	}
}
