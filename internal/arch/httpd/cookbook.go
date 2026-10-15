package httpd

import (
	"net/http"
	"strconv"
	"strings"

	"dasyra/internal/dom"
)

// The cookbook card of the user page: the Mealie server and API token this user's Dasyra works with.
// Without one the user has no cookbook tools.

type cookbookForm struct {
	Connected      bool
	URL, PublicURL string
	Masked         string // the token is never put back into the page
}

func cookbookFormOf(c *dom.Cookbook) cookbookForm {
	if c == nil {
		return cookbookForm{}
	}
	return cookbookForm{Connected: true, URL: c.URL, PublicURL: c.PublicURL, Masked: c.MaskedToken()}
}

var cookbookNotices = map[string]string{"cookbook": "Cookbook saved, Dasyra uses it from the next reply.",
	"cookbook-deleted": "Cookbook disconnected, Dasyra no longer sees it."}

func (a *Admin) saveCookbook(w http.ResponseWriter, r *http.Request, sess *session) {
	c, problem := a.cookbookFromForm(r, sess)
	if problem == "" {
		if err := a.Cookbooks.SaveCookbook(r.Context(), c); err != nil {
			a.fail(w, err)
			return
		}
		http.Redirect(w, r, "/panel/users/"+r.PathValue("id")+"?saved=cookbook", http.StatusFound)
		return
	}
	a.showCookbookDraft(w, r, sess, http.StatusBadRequest, "", problem, c)
}

// checkCookbook calls the server with what is in the form, saved or not, a blank token meaning the saved one.
func (a *Admin) checkCookbook(w http.ResponseWriter, r *http.Request, sess *session) {
	c, problem := a.cookbookFromForm(r, sess)
	if problem != "" {
		a.showCookbookDraft(w, r, sess, http.StatusBadRequest, "", problem, c)
		return
	}
	who, err := a.CheckCookbook(r.Context(), c)
	if err != nil {
		a.showCookbookDraft(w, r, sess, http.StatusOK, "", "The cookbook did not answer: "+err.Error(), c)
		return
	}
	a.showCookbookDraft(w, r, sess, http.StatusOK, "The cookbook answers: "+who+".", "", c)
}

func (a *Admin) deleteCookbook(w http.ResponseWriter, r *http.Request, sess *session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.showUser(w, r, sess, http.StatusForbidden, func(v *panelView) { v.Error = "The form has expired, please try again." })
		return
	}
	if err := a.Cookbooks.DeleteCookbook(r.Context(), id); err != nil {
		a.fail(w, err)
		return
	}
	http.Redirect(w, r, "/panel/users/"+r.PathValue("id")+"?saved=cookbook-deleted", http.StatusFound)
}

// showCookbookDraft shows the user page with the form as it was sent, so a failed save loses nothing.
func (a *Admin) showCookbookDraft(w http.ResponseWriter, r *http.Request, sess *session, status int, notice, problem string, c dom.Cookbook) {
	a.showUser(w, r, sess, status, func(v *panelView) {
		v.Notice, v.Error = notice, problem
		v.Cookbook.URL, v.Cookbook.PublicURL = c.URL, c.PublicURL
	})
}

// cookbookFromForm reads and checks the cookbook form, the second value is what is wrong with it.
func (a *Admin) cookbookFromForm(r *http.Request, sess *session) (dom.Cookbook, string) {
	c := dom.Cookbook{
		URL:       strings.TrimRight(strings.TrimSpace(r.PostFormValue("url")), "/"),
		PublicURL: strings.TrimRight(strings.TrimSpace(r.PostFormValue("public_url")), "/"),
		Token:     strings.TrimSpace(r.PostFormValue("token")),
	}
	c.UserID, _ = strconv.ParseInt(r.PathValue("id"), 10, 64)
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		return c, "The form has expired, please try again."
	}
	if u, err := a.Users.Get(r.Context(), c.UserID); err != nil || u == nil {
		return c, "This user no longer exists."
	}
	if c.Token == "" {
		if saved, err := a.Cookbooks.Cookbook(r.Context(), c.UserID); err == nil && saved != nil {
			c.Token = saved.Token
		}
	}
	switch {
	case !webURL(c.URL):
		return c, "The API address must start with http:// or https://, for example http://mealie:9000."
	case c.PublicURL != "" && !webURL(c.PublicURL):
		return c, "The public address must start with http:// or https://."
	case c.Token == "":
		return c, "An API token is required: in Mealie, the user's profile → API Tokens."
	}
	return c, ""
}
