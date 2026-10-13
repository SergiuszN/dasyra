package httpd

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// The models page: providers with their keys, which provider and model each role runs on, and the prices of
// models whose provider reports no cost.

type aiView struct {
	Title, CSRF   string
	Providers     []providerRow
	Roles         []roleRow
	Notice, Error string
	Draft         providerForm // the add form, keeps what was typed when it failed
	Efforts       []option
	Prices        []priceRow
}

type providerRow struct {
	dom.AIProvider
	Masked, KindLabel string
	Uses              []string // labels of the roles running on it
	Form              providerForm
}

// providerForm fills the provider fields, the key is never put back into the page.
type providerForm struct {
	ID                          int64
	Name, Kind, BaseURL, Masked string
	Kinds                       []option
}

func formOf(p dom.AIProvider) providerForm {
	f := providerForm{ID: p.ID, Name: p.Name, Kind: p.Kind, BaseURL: p.BaseURL, Kinds: kinds}
	if p.ID > 0 {
		f.Masked = p.MaskedKey()
	}
	return f
}

type roleRow struct {
	Key, Label, Hint string
	Default          llm.Model
	Set              dom.AIRole
	NoEffort         bool // transcription and embeddings have none
}

type option struct{ Value, Label string }

var kinds = []option{{dom.ProviderOpenRouter, "OpenRouter"}, {dom.ProviderOpenAI, "OpenAI-compatible"}}

var efforts = []option{{"none", "none"}, {"low", "low"}, {"medium", "medium"}, {"high", "high"}, {"xhigh", "xhigh"}}

var roleInfo = map[string][2]string{
	llm.RoleDasyra:        {"Dasyra: chat", "replies to the user"},
	llm.RoleCheckIn:       {"Dasyra: check-ins", "writing first at the check-in hours"},
	llm.RoleReminder:      {"Dasyra: reminders", "due events, digests, regular tasks"},
	llm.RoleReflection:    {"Reflection", "nightly memory upkeep"},
	llm.RoleWebSearch:     {"Web search", "answer synthesis in web_search answer/research"},
	llm.RoleTextReader:    {"Text reader", "pages and documents for web_fetch and read_document"},
	llm.RoleTranscription: {"Voice transcription", "the /audio/transcriptions endpoint"},
	llm.RoleEmbedding:     {"Embeddings", "search by meaning, the /embeddings endpoint"},
	llm.RoleCheckInGate:   {"Check-in gate", "a decision model, asked before a check-in when the gate is on"},
}

// Notices after a redirect, by the saved query value.
var notices = map[string]string{"provider": "Provider saved.", "deleted": "Provider deleted, its roles and prices were reset.",
	"roles": "Roles saved, they apply from the next call.", "prices": "Prices saved, they count from the next call."}

func (a *Admin) aiPage(w http.ResponseWriter, r *http.Request, sess *session) {
	a.renderAI(w, r, sess, http.StatusOK, notices[r.URL.Query().Get("saved")], "")
}

func (a *Admin) renderAI(w http.ResponseWriter, r *http.Request, sess *session, status int, notice, problem string) {
	a.showAI(w, r, sess, status, func(v *aiView) { v.Notice, v.Error = notice, problem })
}

// renderAIDraft shows the page with a new provider's form filled as it was sent.
func (a *Admin) renderAIDraft(w http.ResponseWriter, r *http.Request, sess *session, status int, notice, problem string, draft dom.AIProvider) {
	a.showAI(w, r, sess, status, func(v *aiView) {
		v.Notice, v.Error = notice, problem
		if draft.ID == 0 {
			v.Draft = formOf(draft)
		}
	})
}

// showAI loads the page, edit sets what the request adds to it.
func (a *Admin) showAI(w http.ResponseWriter, r *http.Request, sess *session, status int, edit func(*aiView)) {
	ctx := r.Context()
	providers, err := a.AI.Providers(ctx)
	if err != nil {
		a.fail(w, err)
		return
	}
	roles, err := a.AI.Roles(ctx)
	if err != nil {
		a.fail(w, err)
		return
	}
	prices, err := a.AI.Prices(ctx)
	if err != nil {
		a.fail(w, err)
		return
	}
	v := aiView{Title: "Models", CSRF: sess.csrf, Draft: formOf(dom.AIProvider{}), Efforts: efforts, Prices: priceRows(providers, roles, prices)}
	for _, p := range providers {
		row := providerRow{AIProvider: p, Masked: p.MaskedKey(), KindLabel: kindLabel(p.Kind), Form: formOf(p)}
		for _, m := range llm.Models {
			if roles[m.Role].ProviderID == p.ID {
				row.Uses = append(row.Uses, roleInfo[m.Role][0])
			}
		}
		row.Key = "" // never reaches the page
		v.Providers = append(v.Providers, row)
	}
	for _, m := range llm.Models {
		info := roleInfo[m.Role]
		v.Roles = append(v.Roles, roleRow{Key: m.Role, Label: info[0], Hint: info[1], Default: m, Set: roles[m.Role],
			NoEffort: m.Role == llm.RoleTranscription || m.Role == llm.RoleEmbedding || m.Role == llm.RoleCheckInGate})
	}
	edit(&v)
	a.render(w, "ai", status, v)
}

func (a *Admin) saveProvider(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	p, problem := a.providerFromForm(r)
	if problem == "" {
		if err := a.AI.SaveProvider(r.Context(), &p); err != nil {
			a.fail(w, err)
			return
		}
		http.Redirect(w, r, "/panel/ai?saved=provider", http.StatusFound)
		return
	}
	a.renderAIDraft(w, r, sess, http.StatusBadRequest, "", problem, p)
}

// checkProvider calls the provider with what is in the form, saved or not, a blank key meaning the saved one.
func (a *Admin) checkProvider(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	p, problem := a.providerFromForm(r)
	if problem != "" {
		a.renderAIDraft(w, r, sess, http.StatusBadRequest, "", problem, p)
		return
	}
	if err := a.CheckAI(r.Context(), p); err != nil {
		a.renderAIDraft(w, r, sess, http.StatusOK, "", p.Name+": no answer: "+err.Error(), p)
		return
	}
	notice := p.Name + ": key accepted, the provider answers."
	if p.Kind != dom.ProviderOpenRouter {
		notice = p.Name + ": answers /models (some APIs list their models without a key too)."
	}
	a.renderAIDraft(w, r, sess, http.StatusOK, notice, "", p)
}

func (a *Admin) deleteProvider(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	id, _ := strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if err := a.AI.DeleteProvider(r.Context(), id); err != nil {
		a.fail(w, err)
		return
	}
	http.Redirect(w, r, "/panel/ai?saved=deleted", http.StatusFound)
}

// providerFromForm reads and checks the provider form, the second value is what is wrong with it.
func (a *Admin) providerFromForm(r *http.Request) (dom.AIProvider, string) {
	p := dom.AIProvider{
		Name:    strings.TrimSpace(r.PostFormValue("name")),
		Kind:    r.PostFormValue("kind"),
		BaseURL: strings.TrimRight(strings.TrimSpace(r.PostFormValue("base_url")), "/"),
		Key:     strings.TrimSpace(r.PostFormValue("key")),
	}
	p.ID, _ = strconv.ParseInt(r.PostFormValue("id"), 10, 64)
	if p.ID > 0 {
		saved, err := a.AI.Provider(r.Context(), p.ID)
		if err != nil || saved == nil {
			return p, "This provider no longer exists."
		}
		if p.Key == "" {
			p.Key = saved.Key
		}
	}
	switch {
	case p.Name == "" || len([]rune(p.Name)) > 64:
		return p, "A name is required, up to 64 characters."
	case kindLabel(p.Kind) == "":
		return p, "Unknown provider type."
	case p.Kind == dom.ProviderOpenAI && p.BaseURL == "":
		return p, "An OpenAI-compatible provider needs a base URL, for example https://api.openai.com/v1."
	case p.BaseURL != "" && !webURL(p.BaseURL):
		return p, "The base URL must start with http:// or https://."
	case p.Kind == dom.ProviderOpenRouter && p.Key == "":
		return p, "OpenRouter needs a key."
	}
	return p, ""
}

func (a *Admin) saveRoles(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	ctx := r.Context()
	var roles []dom.AIRole
	for _, m := range llm.Models {
		role, problem := a.roleFromForm(ctx, r, m)
		if problem != "" {
			a.renderAI(w, r, sess, http.StatusBadRequest, "", roleInfo[m.Role][0]+": "+problem)
			return
		}
		roles = append(roles, role)
	}
	// Checked all first, so a wrong row saves nothing
	for _, role := range roles {
		if err := a.AI.SaveRole(ctx, role); err != nil {
			a.fail(w, err)
			return
		}
	}
	http.Redirect(w, r, "/panel/ai?saved=roles", http.StatusFound)
}

func (a *Admin) roleFromForm(ctx context.Context, r *http.Request, m llm.Model) (dom.AIRole, string) {
	role := dom.AIRole{Role: m.Role, Model: strings.TrimSpace(r.PostFormValue("model_" + m.Role)), Effort: r.PostFormValue("effort_" + m.Role)}
	role.ProviderID, _ = strconv.ParseInt(r.PostFormValue("provider_"+m.Role), 10, 64)
	if len(role.Model) > 128 {
		return role, "the model name is too long."
	}
	if role.Effort != "" && !known(efforts, role.Effort) {
		return role, "unknown effort."
	}
	if role.ProviderID > 0 {
		p, err := a.AI.Provider(ctx, role.ProviderID)
		if err != nil || p == nil {
			return role, "this provider no longer exists."
		}
		if m.Role == llm.RoleCheckInGate && p.Kind != dom.ProviderOpenRouter {
			return role, "only OpenRouter serves decision models."
		}
		// The default ids are OpenRouter's (vendor/model), another API would not know them
		if p.Kind != dom.ProviderOpenRouter && role.Model == "" {
			return role, "set a model for the provider " + p.Name + "."
		}
	}
	return role, ""
}

func kindLabel(kind string) string {
	for _, k := range kinds {
		if k.Value == kind {
			return k.Label
		}
	}
	return ""
}

func known(options []option, value string) bool {
	for _, o := range options {
		if o.Value == value {
			return true
		}
	}
	return false
}

func webURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
