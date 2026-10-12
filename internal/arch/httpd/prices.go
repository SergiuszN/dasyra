package httpd

import (
	"net/http"
	"strconv"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// priceRow is one model of a provider that reports no cost: every such model a role runs on, and any price
// saved earlier.
type priceRow struct {
	ProviderID                                      int64
	Provider, Model                                 string
	Audio                                           bool // transcription, priced per minute
	Saved                                           bool
	Input, CacheRead, CacheWrite, Output, PerMinute string
}

func priceRows(providers []dom.AIProvider, roles map[string]dom.AIRole, prices []dom.AIPrice) []priceRow {
	names := map[int64]dom.AIProvider{}
	for _, p := range providers {
		names[p.ID] = p
	}
	saved := map[[2]string]dom.AIPrice{}
	for _, p := range prices {
		saved[priceKey(p.ProviderID, p.Model)] = p
	}
	var rows []priceRow
	seen := map[[2]string]bool{}
	add := func(providerID int64, model string, audio bool) {
		key := priceKey(providerID, model)
		p, ok := names[providerID]
		if seen[key] || !ok {
			return
		}
		seen[key] = true
		row := priceRow{ProviderID: providerID, Provider: p.Name, Model: model, Audio: audio}
		if price, ok := saved[key]; ok {
			row.Saved, row.Audio = true, audio || price.PerMinute > 0
			row.fill(price)
		}
		rows = append(rows, row)
	}
	for _, m := range llm.Models {
		role := roles[m.Role]
		if p, ok := names[role.ProviderID]; ok && p.Kind != dom.ProviderOpenRouter && role.Model != "" {
			add(role.ProviderID, role.Model, m.Role == llm.RoleTranscription)
		}
	}
	for _, p := range prices {
		add(p.ProviderID, p.Model, p.PerMinute > 0)
	}
	return rows
}

func priceKey(providerID int64, model string) [2]string {
	return [2]string{strconv.FormatInt(providerID, 10), model}
}

func (row *priceRow) fill(p dom.AIPrice) {
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	row.Input, row.CacheRead, row.CacheWrite, row.Output, row.PerMinute = f(p.Input), f(p.CacheRead), f(p.CacheWrite), f(p.Output), f(p.PerMinute)
}

// savePrices stores every row of the form, a row left empty drops its price.
func (a *Admin) savePrices(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	ctx := r.Context()
	var save []dom.AIPrice
	var drop []dom.AIPrice
	for i := 0; r.PostFormValue(field("model", i)) != ""; i++ {
		p := dom.AIPrice{Model: r.PostFormValue(field("model", i))}
		p.ProviderID, _ = strconv.ParseInt(r.PostFormValue(field("provider", i)), 10, 64)
		if provider, err := a.AI.Provider(ctx, p.ProviderID); err != nil || provider == nil {
			a.renderAI(w, r, sess, http.StatusBadRequest, "", p.Model+": this provider no longer exists.")
			return
		}
		empty := true
		for _, f := range []struct {
			name string
			to   *float64
		}{{"input", &p.Input}, {"cache_read", &p.CacheRead}, {"cache_write", &p.CacheWrite}, {"output", &p.Output}, {"per_minute", &p.PerMinute}} {
			raw := strings.ReplaceAll(strings.TrimSpace(r.PostFormValue(field(f.name, i))), ",", ".")
			if raw == "" {
				continue
			}
			v, err := strconv.ParseFloat(raw, 64)
			if err != nil || v < 0 || v > 10000 {
				a.renderAI(w, r, sess, http.StatusBadRequest, "", p.Model+": a price must be a number from 0, in US dollars per 1M tokens or per minute.")
				return
			}
			*f.to, empty = v, false
		}
		if empty {
			drop = append(drop, p)
		} else {
			save = append(save, p)
		}
	}
	// Checked all first, so a wrong row saves nothing
	for _, p := range save {
		if err := a.AI.SavePrice(ctx, p); err != nil {
			a.fail(w, err)
			return
		}
	}
	for _, p := range drop {
		if err := a.AI.DeletePrice(ctx, p.ProviderID, p.Model); err != nil {
			a.fail(w, err)
			return
		}
	}
	http.Redirect(w, r, "/panel/ai?saved=prices", http.StatusFound)
}

// suggestPrices fills the rows without a saved price from OpenRouter's catalog, without saving them.
func (a *Admin) suggestPrices(w http.ResponseWriter, r *http.Request, sess *session) {
	if !sess.validCSRF(r.PostFormValue("csrf")) {
		a.renderAI(w, r, sess, http.StatusForbidden, "", "The form has expired, please try again.")
		return
	}
	a.showAI(w, r, sess, http.StatusOK, func(v *aiView) {
		var models []string
		for _, row := range v.Prices {
			if !row.Saved && !row.Audio {
				models = append(models, row.Model)
			}
		}
		if len(models) == 0 {
			v.Notice = "Nothing to fill in: every model already has a saved price (OpenRouter doesn't sell transcription, enter its price by hand)."
			return
		}
		found, err := a.SuggestPrices(r.Context(), models)
		if err != nil {
			v.Error = "The OpenRouter catalog did not answer: " + err.Error()
			return
		}
		for i, row := range v.Prices {
			if p, ok := found[row.Model]; ok && !row.Saved && !row.Audio {
				v.Prices[i].fill(p)
				v.Prices[i].PerMinute = ""
			}
		}
		v.Notice = "Found in the OpenRouter catalog: " + strconv.Itoa(len(found)) + " of " + strconv.Itoa(len(models)) +
			". These are OpenRouter prices, the provider's own may differ: check them and save."
	})
}

func field(name string, i int) string { return "price_" + strconv.Itoa(i) + "_" + name }
