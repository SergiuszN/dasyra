package httpd

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookie = "dasyra_session"
	sessionTTL    = 7 * 24 * time.Hour
)

// session lives in memory: the app is one long-running process, a restart only logs the admin out.
type session struct {
	id      string
	csrf    string
	login   string // empty while not logged in
	expires time.Time
}

func (s *session) validCSRF(token string) bool {
	return subtle.ConstantTimeCompare([]byte(s.csrf), []byte(token)) == 1
}

type sessions struct {
	mu   sync.Mutex
	byID map[string]*session
}

// get returns the request's session, starting a new one (and setting its cookie) when there is none.
func (s *sessions) get(w http.ResponseWriter, r *http.Request) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, err := r.Cookie(sessionCookie); err == nil {
		if sess, ok := s.byID[c.Value]; ok && time.Now().Before(sess.expires) {
			return sess
		}
	}
	return s.start(w, r, "")
}

// login gives the session a fresh id, so one planted before the login is worthless, and a fresh CSRF token.
func (s *sessions) login(w http.ResponseWriter, r *http.Request, old *session, login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.byID, old.id)
	s.start(w, r, login)
}

func (s *sessions) logout(sess *session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess.login = ""
}

func (s *sessions) start(w http.ResponseWriter, r *http.Request, login string) *session {
	if s.byID == nil {
		s.byID = map[string]*session{}
	}
	now := time.Now()
	for id, sess := range s.byID {
		if now.After(sess.expires) {
			delete(s.byID, id)
		}
	}
	sess := &session{id: token(), csrf: token(), login: login, expires: now.Add(sessionTTL)}
	s.byID[sess.id] = sess
	// Behind the Cloudflare tunnel the app sees plain http, the original scheme comes in a header
	secure := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sess.id, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode, Expires: sess.expires})
	return sess
}

func token() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
