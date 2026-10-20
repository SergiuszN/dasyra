package httpd

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"dasyra/internal/dom"
)

// The users page: who has access, turning a user off and back on, and invite links for new ones.

type usersView struct {
	Title, CSRF   string
	Notice, Error string
	Users         []*dom.User
	Open, Used    []inviteRow
}

type inviteRow struct {
	dom.Invite
	Link string // t.me link, or the /start command when the bot name is unknown
	User string // who used it
}

var usersNotices = map[string]string{"invite": "Invite created: send the link to the person, it works once.",
	"revoked": "Invite revoked.", "off": "Access turned off: Dasyra no longer answers or writes to this user, their data is kept.",
	"on": "Access is back: Dasyra answers this user again, check-ins resume."}

func (a *Admin) usersPage(w http.ResponseWriter, r *http.Request, sess *session) {
	a.renderUsers(w, r, sess, http.StatusOK, usersNotices[r.URL.Query().Get("saved")], "")
}

func (a *Admin) renderUsers(w http.ResponseWriter, r *http.Request, sess *session, status int, notice, problem string) {
	ctx := r.Context()
	users, err := a.Users.All(ctx)
	invites, err2 := a.Invites.All(ctx)
	if err = firstErr(err, err2); err != nil {
		a.fail(w, err)
		return
	}
	names := map[int64]string{}
	for _, u := range users {
		names[u.ID] = nameOr(u.Name)
	}
	bot := ""
	if a.BotName != nil {
		bot, _ = a.BotName(ctx)
	}
	v := usersView{Title: "Users", CSRF: sess.csrf, Notice: notice, Error: problem, Users: users}
	for _, i := range invites {
		row := inviteRow{Invite: i, Link: "/start " + i.Code, User: names[i.UserID]}
		if bot != "" {
			row.Link = "https://t.me/" + bot + "?start=" + i.Code
		}
		if i.Used() {
			v.Used = append(v.Used, row)
		} else {
			v.Open = append(v.Open, row)
		}
	}
	a.render(w, "users", status, v)
}

func (a *Admin) createInvite(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderUsers(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	if err := a.Invites.Create(r.Context(), dom.NewInviteCode(), strings.TrimSpace(r.PostFormValue("note"))); err != nil {
		a.fail(w, err)
		return
	}
	http.Redirect(w, r, "/panel/users?saved=invite", http.StatusFound)
}

func (a *Admin) revokeInvite(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderUsers(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	if err := a.Invites.Revoke(r.Context(), r.PostFormValue("code")); err != nil {
		a.fail(w, err)
		return
	}
	http.Redirect(w, r, "/panel/users?saved=revoked", http.StatusFound)
}

// setAccess turns a user off (access=off) or back on; the owner can't be turned off.
func (a *Admin) setAccess(w http.ResponseWriter, r *http.Request, sess *session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderUsers(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	if u, err := a.Users.Get(r.Context(), id); err != nil || u == nil {
		http.NotFound(w, r)
		return
	}
	off := r.PostFormValue("access") == "off"
	if off && id == dom.OwnerID {
		a.renderUsers(w, r, sess, http.StatusBadRequest, "", "The owner cannot be turned off.")
		return
	}
	var at time.Time
	if off {
		at = time.Now()
	}
	if err := a.Users.SetDisabled(r.Context(), id, at); err != nil {
		a.fail(w, err)
		return
	}
	a.Log.Info("User access changed", "user_id", id, "off", off)
	saved := "on"
	if off {
		saved = "off"
	}
	http.Redirect(w, r, "/panel/users?saved="+saved, http.StatusFound)
}
