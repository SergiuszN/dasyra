// Package httpd serves the Telegram webhook and the admin panel.
package httpd

import (
	"context"
	"crypto/subtle"
	"embed"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

// UsageReport is the AI cost accounting the panel shows, implemented by sqlite.Report.
type UsageReport interface {
	Totals(ctx context.Context, from *time.Time, user *int64) (sqlite.Totals, error)
	ByUser(ctx context.Context, from *time.Time) ([]sqlite.Group, error)
	Grouped(ctx context.Context, column string, from *time.Time, user *int64) ([]sqlite.Group, error)
	Daily(ctx context.Context, from *time.Time, user *int64) (map[string]sqlite.Totals, error)
	RepliesByUser(ctx context.Context, from time.Time) (map[int64]int, error)
	FirstAt(ctx context.Context) (time.Time, error)
}

// Admin is the panel with AI costs and tokens, overall and per user, and the models each role runs on.
// It stays closed while Login or Password is empty.
type Admin struct {
	Report  UsageReport
	Users   dom.Users
	AI      dom.AISettings
	Plans   dom.CheckInPlans
	CheckAI func(ctx context.Context, p dom.AIProvider) error // whether the provider answers with its key
	// SuggestPrices finds models in OpenRouter's catalog, a hint for the prices form
	SuggestPrices   func(ctx context.Context, models []string) (map[string]dom.AIPrice, error)
	Login, Password string
	Log             *slog.Logger

	sessions sessions
	pause    time.Duration // after a failed login
	once     sync.Once
}

//go:embed templates/*.html
var templates embed.FS

var pages = template.Must(template.New("").Funcs(funcs).ParseFS(templates, "templates/*.html"))

const defaultPeriod = "30"

// Days shown in the period switch, "all" is all time.
var periods = []struct{ Key, Label string }{{"1", "Today"}, {"7", "7 days"}, {"30", "30 days"}, {"all", "All time"}}

func (a *Admin) Register(mux *http.ServeMux) {
	a.once.Do(func() {
		if a.pause == 0 {
			a.pause = 2 * time.Second
		}
	})
	mux.HandleFunc("GET /login", a.loginForm)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", a.logout)
	mux.HandleFunc("GET /panel", a.guard(a.dashboard))
	mux.HandleFunc("GET /panel/users/{id}", a.guard(a.user))
	mux.HandleFunc("GET /panel/ai", a.guard(a.aiPage))
	mux.HandleFunc("POST /panel/ai/providers", a.guard(a.saveProvider))
	mux.HandleFunc("POST /panel/ai/providers/check", a.guard(a.checkProvider))
	mux.HandleFunc("POST /panel/ai/providers/delete", a.guard(a.deleteProvider))
	mux.HandleFunc("POST /panel/ai/roles", a.guard(a.saveRoles))
	mux.HandleFunc("POST /panel/ai/prices", a.guard(a.savePrices))
	mux.HandleFunc("POST /panel/ai/prices/suggest", a.guard(a.suggestPrices))
	mux.HandleFunc("GET /panel/check-ins", a.guard(a.checkInsPage))
	mux.HandleFunc("POST /panel/check-ins", a.guard(a.saveCheckIns))
}

func (a *Admin) configured() bool { return a.Login != "" && a.Password != "" }

func (a *Admin) guard(next func(http.ResponseWriter, *http.Request, *session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sess := a.sessions.get(w, r)
		if sess.login == "" || !a.configured() {
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}
		next(w, r, sess)
	}
}

func (a *Admin) loginForm(w http.ResponseWriter, r *http.Request) {
	sess := a.sessions.get(w, r)
	if sess.login != "" && a.configured() {
		http.Redirect(w, r, "/panel", http.StatusFound)
		return
	}
	a.render(w, "login", http.StatusOK, loginView{CSRF: sess.csrf, Configured: a.configured()})
}

func (a *Admin) login(w http.ResponseWriter, r *http.Request) {
	sess := a.sessions.get(w, r)
	if msg := a.check(sess, r.PostFormValue("login"), r.PostFormValue("password"), r.PostFormValue("csrf")); msg != "" {
		a.render(w, "login", http.StatusUnauthorized, loginView{CSRF: sess.csrf, Configured: a.configured(), Error: msg})
		return
	}
	a.sessions.login(w, r, sess, a.Login)
	http.Redirect(w, r, "/panel", http.StatusFound)
}

// check returns why the login failed, "" when it succeeded.
func (a *Admin) check(sess *session, login, password, csrf string) string {
	if !a.configured() {
		return "Login is closed: ADMIN_LOGIN and ADMIN_PASSWORD are not set."
	}
	if !sess.validCSRF(csrf) {
		return "The form has expired, please try again."
	}
	// Both compared every time, so the answer time does not tell which one was wrong
	loginOK := subtle.ConstantTimeCompare([]byte(a.Login), []byte(login))
	passwordOK := subtle.ConstantTimeCompare([]byte(a.Password), []byte(password))
	if loginOK&passwordOK != 1 {
		time.Sleep(a.pause)
		return "Wrong login or password."
	}
	return ""
}

func (a *Admin) logout(w http.ResponseWriter, r *http.Request) {
	if sess := a.sessions.get(w, r); sess.validCSRF(r.PostFormValue("csrf")) {
		a.sessions.logout(sess)
	}
	http.Redirect(w, r, "/login", http.StatusFound)
}

func (a *Admin) dashboard(w http.ResponseWriter, r *http.Request, sess *session) {
	ctx, period, from := r.Context(), periodOf(r), periodStart(periodOf(r))
	v := a.panel(r, sess, "AI spend", period)
	users, err := a.Report.ByUser(ctx, from)
	if err == nil {
		err = a.fill(ctx, &v, from, nil)
	}
	replies, err2 := a.Report.RepliesByUser(ctx, a.since(ctx, from))
	if err = firstErr(err, err2); err != nil {
		a.fail(w, err)
		return
	}
	v.Users, v.Replies, v.ShowReplies = userRows(users, replies), sumReplies(replies), true
	if first, _ := a.Report.FirstAt(ctx); !first.IsZero() {
		v.TrackedSince = first.Format("2 Jan 2006 15:04")
	}
	a.render(w, "dashboard", http.StatusOK, v)
}

func (a *Admin) user(w http.ResponseWriter, r *http.Request, sess *session) {
	ctx, period := r.Context(), periodOf(r)
	// 0 is the bucket of calls made on nobody's behalf
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	var u *dom.User
	if id > 0 {
		if u, _ = a.Users.Get(ctx, id); u == nil {
			http.NotFound(w, r)
			return
		}
	}

	from := periodStart(period)
	title := "No user"
	if u != nil {
		title = nameOr(u.Name)
	}
	v := a.panel(r, sess, title, period)
	v.Crumbs = "/panel?period=" + period
	if err := a.fill(ctx, &v, from, &id); err != nil {
		a.fail(w, err)
		return
	}
	if u != nil {
		replies, _ := a.Report.RepliesByUser(ctx, a.since(ctx, from))
		v.Replies, v.ShowReplies, v.User = replies[id], true, u
	}
	a.render(w, "user", http.StatusOK, v)
}

// fill loads what both pages show: tiles, the daily chart and the breakdowns.
func (a *Admin) fill(ctx context.Context, v *panelView, from *time.Time, user *int64) error {
	totals, e0 := a.Report.Totals(ctx, from, user)
	purposes, e1 := a.Report.Grouped(ctx, "purpose", from, user)
	models, e2 := a.Report.Grouped(ctx, "model", from, user)
	daily, e3 := a.Report.Daily(ctx, from, user)
	v.Totals = totals
	v.Purposes = breakdownOf("By purpose", purposes, purposeLabel)
	v.Models = breakdownOf("By model", models, modelLabel)
	v.Chart = chartOf(a.days(ctx, daily, from))
	return firstErr(e0, e1, e2, e3)
}

func (a *Admin) panel(r *http.Request, sess *session, title, period string) panelView {
	return panelView{Title: title, Path: r.URL.Path, Period: period, CSRF: sess.csrf}
}

// since is the period start, or the start of accounting for all time.
func (a *Admin) since(ctx context.Context, from *time.Time) time.Time {
	if from != nil {
		return *from
	}
	if first, _ := a.Report.FirstAt(ctx); !first.IsZero() {
		return first
	}
	return time.Now()
}

// days lists every day of the period, days without calls included, so the chart keeps its time scale.
// Days before accounting started are not zero, they are unknown.
func (a *Admin) days(ctx context.Context, daily map[string]sqlite.Totals, from *time.Time) []day {
	start := midnight(a.since(ctx, nil))
	if from != nil && from.After(start) {
		start = *from
	}
	var days []day
	for d, today := start, midnight(time.Now()); !d.After(today); d = d.AddDate(0, 0, 1) {
		days = append(days, day{d, daily[d.Format("2006-01-02")].Cost})
	}
	return days
}

func (a *Admin) render(w http.ResponseWriter, name string, status int, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := pages.ExecuteTemplate(w, name, data); err != nil {
		a.Log.Error("Admin template failed", "page", name, "error", err)
	}
}

func (a *Admin) fail(w http.ResponseWriter, err error) {
	a.Log.Error("Admin panel failed", "error", err)
	http.Error(w, "Internal error", http.StatusInternalServerError)
}

func periodOf(r *http.Request) string {
	p := r.URL.Query().Get("period")
	for _, known := range periods {
		if known.Key == p {
			return p
		}
	}
	return defaultPeriod
}

// periodStart is the first midnight of the period, nil for all time.
func periodStart(period string) *time.Time {
	n, err := strconv.Atoi(period)
	if err != nil {
		return nil
	}
	from := midnight(time.Now()).AddDate(0, 0, -(n - 1))
	return &from
}

func midnight(t time.Time) time.Time {
	t = t.In(time.Local)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local)
}

func firstErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}
