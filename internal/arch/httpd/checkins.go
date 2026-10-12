package httpd

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// The check-ins page: at which hours Dasyra writes first, how she backs off from silent users, and the gate
// that asks a decision model before each check-in.

type checkInsView struct {
	Title, CSRF   string
	Notice, Error string
	Plan          dom.CheckInPlan
	Hours         []hourBox
	QuietHour     int
	GateModel     string // set on the models page
}

type hourBox struct {
	Hour int
	On   bool
}

func (a *Admin) checkInsPage(w http.ResponseWriter, r *http.Request, sess *session) {
	plan, err := a.Plans.CheckInPlan(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	notice := ""
	if r.URL.Query().Get("saved") == "1" {
		notice = "Check-ins saved, they apply from the next full hour."
	}
	a.renderCheckIns(w, r, sess, http.StatusOK, plan, notice, "")
}

func (a *Admin) saveCheckIns(w http.ResponseWriter, r *http.Request, sess *session) {
	plan, problem := planFromForm(r)
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		problem = "The form has expired, please try again."
	}
	if problem != "" {
		a.renderCheckIns(w, r, sess, http.StatusBadRequest, plan, "", problem)
		return
	}
	if err := a.Plans.SaveCheckInPlan(r.Context(), plan); err != nil {
		a.fail(w, err)
		return
	}
	http.Redirect(w, r, "/panel/check-ins?saved=1", http.StatusFound)
}

func (a *Admin) renderCheckIns(w http.ResponseWriter, r *http.Request, sess *session, status int, plan dom.CheckInPlan, notice, problem string) {
	roles, err := a.AI.Roles(r.Context())
	if err != nil {
		a.fail(w, err)
		return
	}
	v := checkInsView{Title: "Check-ins", CSRF: sess.csrf, Notice: notice, Error: problem, Plan: plan, QuietHour: plan.QuietHour(), GateModel: llm.CheckInGate.ID}
	if m := roles[llm.RoleCheckInGate].Model; m != "" {
		v.GateModel = m
	}
	for h := range 24 {
		v.Hours = append(v.Hours, hourBox{h, slices.Contains(plan.Hours, h)})
	}
	a.render(w, "checkins", status, v)
}

// planFromForm reads the form, the second value is what is wrong with it.
func planFromForm(r *http.Request) (dom.CheckInPlan, string) {
	_ = r.ParseForm()
	plan := dom.CheckInPlan{Hours: []int{}, Gate: r.PostFormValue("gate") == "on"}
	for _, v := range r.PostForm["hour"] {
		h, err := strconv.Atoi(v)
		if err != nil {
			return plan, "Unknown hour."
		}
		plan.Hours = append(plan.Hours, h)
	}
	var e1, e2, e3 error
	plan.QuietDays, e1 = strconv.Atoi(strings.TrimSpace(r.PostFormValue("quiet_days")))
	plan.StopDays, e2 = strconv.Atoi(strings.TrimSpace(r.PostFormValue("stop_days")))
	plan.Threshold, e3 = strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.PostFormValue("threshold")), ",", "."), 64)
	if firstErr(e1, e2, e3) != nil {
		return plan, "Days are whole numbers, the threshold a number such as 0.3."
	}
	if problem := plan.Problem(); problem != "" {
		return plan, strings.ToUpper(problem[:1]) + problem[1:]
	}
	return plan, ""
}
