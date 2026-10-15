// Package mealie talks to Mealie (https://mealie.io), the self-hosted cookbook a user can connect in the admin panel.
// Every call goes with that user's API token, so it sees their household's recipes, meal plan and shopping lists.
package mealie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"dasyra/internal/dom"
)

// Client is stateless but for the group slug each token's recipe links need.
type Client struct {
	HTTP   *http.Client
	groups sync.Map // token → group slug
}

func New() *Client {
	// Importing a recipe page makes Mealie fetch and parse it, which takes a while
	return &Client{HTTP: &http.Client{Timeout: 60 * time.Second}}
}

// ErrNotFound is a recipe, an entry or a list that is not there.
var ErrNotFound = errors.New("not found in the cookbook")

// call sends body as JSON (when not nil) and decodes the answer into out (when not nil).
func (c *Client) call(ctx context.Context, cb dom.Cookbook, method, path string, query url.Values, body, out any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	target := strings.TrimRight(cb.URL, "/") + path
	if len(query) > 0 {
		target += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cb.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the cookbook refused the token (HTTP %d)", resp.StatusCode)
	case resp.StatusCode >= 300:
		return fmt.Errorf("cookbook HTTP %d: %s", resp.StatusCode, detail(data))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(data, out)
}

// detail is the readable part of Mealie's error answer.
func detail(data []byte) string {
	var e struct {
		Detail any `json:"detail"`
	}
	if json.Unmarshal(data, &e) == nil && e.Detail != nil {
		if m, ok := e.Detail.(map[string]any); ok && m["message"] != nil {
			return fmt.Sprint(m["message"])
		}
		return fmt.Sprint(e.Detail)
	}
	text := []rune(strings.TrimSpace(string(data)))
	return string(text[:min(len(text), 200)])
}

type self struct {
	Username  string `json:"username"`
	FullName  string `json:"fullName"`
	Group     string `json:"group"`
	GroupSlug string `json:"groupSlug"`
	Household string `json:"household"`
}

// Check says whose the token is, for the admin panel.
func (c *Client) Check(ctx context.Context, cb dom.Cookbook) (string, error) {
	var me self
	if err := c.call(ctx, cb, http.MethodGet, "/api/users/self", nil, nil, &me); err != nil {
		return "", err
	}
	c.groups.Store(cb.Token, me.GroupSlug)
	name := me.FullName
	if name == "" {
		name = me.Username
	}
	return fmt.Sprintf("user %s, household %s", name, me.Household), nil
}

// link is where the user opens the recipe, "" without a public URL.
func (c *Client) link(ctx context.Context, cb dom.Cookbook, slug string) string {
	if cb.PublicURL == "" || slug == "" {
		return ""
	}
	group, ok := c.groups.Load(cb.Token)
	if !ok {
		var me self
		if c.call(ctx, cb, http.MethodGet, "/api/users/self", nil, nil, &me) != nil || me.GroupSlug == "" {
			return ""
		}
		c.groups.Store(cb.Token, me.GroupSlug)
		group = me.GroupSlug
	}
	return strings.TrimRight(cb.PublicURL, "/") + "/g/" + group.(string) + "/r/" + slug
}

type tag struct {
	ID      string `json:"id"`
	GroupID string `json:"groupId,omitempty"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
}

type recipe struct {
	ID               string  `json:"id"`
	Slug             string  `json:"slug"`
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	RecipeYield      string  `json:"recipeYield"`
	RecipeServings   float64 `json:"recipeServings"`
	TotalTime        *string `json:"totalTime"`
	Tags             []tag   `json:"tags"`
	OrgURL           string  `json:"orgURL"`
	RecipeIngredient []struct {
		Display string `json:"display"`
		Note    string `json:"note"`
		Title   string `json:"title"`
	} `json:"recipeIngredient"`
	RecipeInstructions []struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"recipeInstructions"`
	Notes []struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"notes"`
}

func (r recipe) dom() dom.Recipe {
	out := dom.Recipe{ID: r.ID, Slug: r.Slug, Name: r.Name, Description: r.Description, Servings: r.RecipeYield, Source: r.OrgURL}
	if out.Servings == "" && r.RecipeServings > 0 {
		out.Servings = strconv.FormatFloat(r.RecipeServings, 'f', -1, 64)
	}
	if r.TotalTime != nil {
		out.TotalTime = *r.TotalTime
	}
	for _, t := range r.Tags {
		out.Tags = append(out.Tags, t.Name)
	}
	for _, i := range r.RecipeIngredient {
		line := strings.TrimSpace(i.Display)
		if line == "" {
			line = strings.TrimSpace(i.Note)
		}
		if i.Title != "" {
			out.Ingredients = append(out.Ingredients, "## "+i.Title)
		}
		if line != "" {
			out.Ingredients = append(out.Ingredients, line)
		}
	}
	for _, s := range r.RecipeInstructions {
		text := strings.TrimSpace(s.Text)
		if s.Title != "" {
			text = s.Title + ": " + text
		}
		out.Steps = append(out.Steps, text)
	}
	for _, n := range r.Notes {
		out.Notes = append(out.Notes, strings.TrimSpace(strings.TrimSpace(n.Title+"\n"+n.Text)))
	}
	return out
}

type page[T any] struct {
	Items []T `json:"items"`
}

// Search lists recipes matching the query and every tag, alphabetically without a query.
func (c *Client) Search(ctx context.Context, cb dom.Cookbook, query string, tags []string, offset, limit int) ([]dom.Recipe, error) {
	q := url.Values{"page": {strconv.Itoa(offset/limit + 1)}, "perPage": {strconv.Itoa(limit)}}
	if query != "" {
		q.Set("search", query)
	} else {
		q.Set("orderBy", "name")
		q.Set("orderDirection", "asc")
	}
	for _, name := range tags {
		t, err := c.findTag(ctx, cb, name)
		if err != nil {
			return nil, err
		}
		if t == nil {
			return nil, nil
		}
		q.Add("tags", t.ID)
	}
	if len(tags) > 1 {
		q.Set("requireAllTags", "true")
	}
	var found page[recipe]
	if err := c.call(ctx, cb, http.MethodGet, "/api/recipes", q, nil, &found); err != nil {
		return nil, err
	}
	out := make([]dom.Recipe, len(found.Items))
	for i, r := range found.Items {
		out[i] = r.dom()
	}
	return out, nil
}

// Recipe is the recipe in full, nil when there is none.
func (c *Client) Recipe(ctx context.Context, cb dom.Cookbook, slug string) (*dom.Recipe, error) {
	var r recipe
	err := c.call(ctx, cb, http.MethodGet, "/api/recipes/"+url.PathEscape(slug), nil, nil, &r)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := r.dom()
	out.Link = c.link(ctx, cb, out.Slug)
	return &out, nil
}

// Import makes Mealie read a recipe page itself and returns the new recipe's slug.
func (c *Client) Import(ctx context.Context, cb dom.Cookbook, pageURL string) (string, error) {
	var slug string
	err := c.call(ctx, cb, http.MethodPost, "/api/recipes/create/url", nil, map[string]any{"url": pageURL, "includeTags": true}, &slug)
	return slug, err
}

// Create saves a recipe written by the model. It goes in as schema.org JSON-LD, the format Mealie reads recipe
// pages in, so it parses the lines and creates the tags itself.
func (c *Client) Create(ctx context.Context, cb dom.Cookbook, r dom.Recipe) (string, error) {
	ld := map[string]any{"@context": "https://schema.org", "@type": "Recipe", "name": r.Name, "recipeIngredient": nonNil(r.Ingredients)}
	steps := make([]map[string]string, len(r.Steps))
	for i, s := range r.Steps {
		steps[i] = map[string]string{"@type": "HowToStep", "text": s}
	}
	ld["recipeInstructions"] = steps
	if r.Description != "" {
		ld["description"] = r.Description
	}
	if r.Servings != "" {
		ld["recipeYield"] = r.Servings
	}
	if len(r.Tags) > 0 {
		ld["keywords"] = strings.Join(r.Tags, ", ")
	}
	data, err := json.Marshal(ld)
	if err != nil {
		return "", err
	}
	body := map[string]any{"data": string(data), "includeTags": true}
	if r.Source != "" {
		body["url"] = r.Source
	}
	var slug string
	if err := c.call(ctx, cb, http.MethodPost, "/api/recipes/create/html-or-json", nil, body, &slug); err != nil {
		return "", err
	}
	// Mealie reads only ISO 8601 durations from JSON-LD, the model writes "40 min"
	if r.TotalTime != "" {
		return c.Update(ctx, cb, slug, dom.Recipe{TotalTime: r.TotalTime})
	}
	return slug, nil
}

// Update changes the fields that are set and returns the slug, new when the name changed.
// Ingredients, steps and tags replace the old ones whole.
func (c *Client) Update(ctx context.Context, cb dom.Cookbook, slug string, r dom.Recipe) (string, error) {
	patch := map[string]any{}
	set := func(key, value string) {
		if value != "" {
			patch[key] = value
		}
	}
	set("name", r.Name)
	set("description", r.Description)
	set("recipeYield", r.Servings)
	set("totalTime", r.TotalTime)
	set("orgURL", r.Source)
	if r.Ingredients != nil {
		lines := make([]map[string]any, len(r.Ingredients))
		for i, line := range r.Ingredients {
			lines[i] = map[string]any{"note": line, "quantity": 0}
		}
		patch["recipeIngredient"] = lines
	}
	if r.Steps != nil {
		steps := make([]map[string]string, len(r.Steps))
		for i, s := range r.Steps {
			steps[i] = map[string]string{"text": s}
		}
		patch["recipeInstructions"] = steps
	}
	if r.Tags != nil {
		tags := []tag{}
		for _, name := range r.Tags {
			t, err := c.tag(ctx, cb, name)
			if err != nil {
				return "", err
			}
			tags = append(tags, *t)
		}
		patch["tags"] = tags
	}
	if len(patch) == 0 {
		return slug, nil
	}
	var updated recipe
	if err := c.call(ctx, cb, http.MethodPatch, "/api/recipes/"+url.PathEscape(slug), nil, patch, &updated); err != nil {
		return "", err
	}
	return updated.Slug, nil
}

// findTag finds a tag by name ignoring case, nil when there is none.
func (c *Client) findTag(ctx context.Context, cb dom.Cookbook, name string) (*tag, error) {
	var found page[tag]
	if err := c.call(ctx, cb, http.MethodGet, "/api/organizers/tags", url.Values{"search": {name}, "perPage": {"50"}}, nil, &found); err != nil {
		return nil, err
	}
	for _, t := range found.Items {
		if strings.EqualFold(t.Name, name) {
			return &t, nil
		}
	}
	return nil, nil
}

// tag finds a tag by name or creates it.
func (c *Client) tag(ctx context.Context, cb dom.Cookbook, name string) (*tag, error) {
	t, err := c.findTag(ctx, cb, name)
	if err != nil || t != nil {
		return t, err
	}
	t = &tag{}
	return t, c.call(ctx, cb, http.MethodPost, "/api/organizers/tags", nil, map[string]string{"name": name}, t)
}

type planEntry struct {
	ID        int64   `json:"id"`
	Date      string  `json:"date"`
	EntryType string  `json:"entryType"`
	Title     string  `json:"title"`
	Recipe    *recipe `json:"recipe"`
}

// Plan lists the meal plan from one day to another, both included.
func (c *Client) Plan(ctx context.Context, cb dom.Cookbook, from, to string) ([]dom.PlanEntry, error) {
	q := url.Values{"start_date": {from}, "end_date": {to}, "perPage": {"200"}, "orderBy": {"date"}, "orderDirection": {"asc"}}
	var found page[planEntry]
	if err := c.call(ctx, cb, http.MethodGet, "/api/households/mealplans", q, nil, &found); err != nil {
		return nil, err
	}
	out := make([]dom.PlanEntry, len(found.Items))
	for i, e := range found.Items {
		out[i] = dom.PlanEntry{ID: e.ID, Day: e.Date, Meal: e.EntryType, Title: e.Title}
		if e.Recipe != nil {
			out[i].RecipeSlug, out[i].RecipeName = e.Recipe.Slug, e.Recipe.Name
		}
	}
	return out, nil
}

// AddToPlan puts a recipe (by slug) or a plain title on a day.
func (c *Client) AddToPlan(ctx context.Context, cb dom.Cookbook, e dom.PlanEntry) error {
	body := map[string]any{"date": e.Day, "entryType": e.Meal, "title": e.Title}
	if e.RecipeSlug != "" {
		var r recipe
		if err := c.call(ctx, cb, http.MethodGet, "/api/recipes/"+url.PathEscape(e.RecipeSlug), nil, nil, &r); err != nil {
			return fmt.Errorf("recipe %s: %w", e.RecipeSlug, err)
		}
		body["recipeId"] = r.ID
	}
	return c.call(ctx, cb, http.MethodPost, "/api/households/mealplans", nil, body, nil)
}

func (c *Client) RemoveFromPlan(ctx context.Context, cb dom.Cookbook, id int64) error {
	return c.call(ctx, cb, http.MethodDelete, "/api/households/mealplans/"+strconv.FormatInt(id, 10), nil, nil, nil)
}

type list struct {
	ID        string           `json:"id"`
	Name      string           `json:"name"`
	ListItems []map[string]any `json:"listItems"`
}

// ShoppingLists are the household's lists without their items, oldest first.
func (c *Client) ShoppingLists(ctx context.Context, cb dom.Cookbook) ([]dom.ShoppingList, error) {
	q := url.Values{"perPage": {"50"}, "orderBy": {"createdAt"}, "orderDirection": {"asc"}}
	var found page[list]
	if err := c.call(ctx, cb, http.MethodGet, "/api/households/shopping/lists", q, nil, &found); err != nil {
		return nil, err
	}
	out := make([]dom.ShoppingList, len(found.Items))
	for i, l := range found.Items {
		out[i] = dom.ShoppingList{ID: l.ID, Name: l.Name}
	}
	return out, nil
}

func (c *Client) CreateList(ctx context.Context, cb dom.Cookbook, name string) (dom.ShoppingList, error) {
	var l list
	err := c.call(ctx, cb, http.MethodPost, "/api/households/shopping/lists", nil, map[string]string{"name": name}, &l)
	return dom.ShoppingList{ID: l.ID, Name: l.Name}, err
}

// ShoppingList is a list with its items.
func (c *Client) ShoppingList(ctx context.Context, cb dom.Cookbook, id string) (*dom.ShoppingList, error) {
	var l list
	if err := c.call(ctx, cb, http.MethodGet, "/api/households/shopping/lists/"+url.PathEscape(id), nil, nil, &l); err != nil {
		return nil, err
	}
	out := &dom.ShoppingList{ID: l.ID, Name: l.Name}
	for _, item := range l.ListItems {
		// A plain line (no food) is its note: display puts "1 " in front of it when its quantity is 1
		text, _ := item["note"].(string)
		if display, _ := item["display"].(string); item["food"] != nil || text == "" {
			text = display
		}
		done, _ := item["checked"].(bool)
		id, _ := item["id"].(string)
		out.Items = append(out.Items, dom.ShoppingItem{ID: id, Text: strings.TrimSpace(text), Done: done})
	}
	return out, nil
}

// AddToList adds plain lines, each as it is written.
func (c *Client) AddToList(ctx context.Context, cb dom.Cookbook, listID string, lines []string) error {
	items := make([]map[string]any, len(lines))
	for i, line := range lines {
		items[i] = map[string]any{"shoppingListId": listID, "note": line, "quantity": 0}
	}
	return c.call(ctx, cb, http.MethodPost, "/api/households/shopping/items/create-bulk", nil, items, nil)
}

// AddRecipeToList adds every ingredient of the recipe, Mealie merges them with what is on the list.
func (c *Client) AddRecipeToList(ctx context.Context, cb dom.Cookbook, listID, slug string) error {
	var r recipe
	if err := c.call(ctx, cb, http.MethodGet, "/api/recipes/"+url.PathEscape(slug), nil, nil, &r); err != nil {
		return fmt.Errorf("recipe %s: %w", slug, err)
	}
	return c.call(ctx, cb, http.MethodPost, "/api/households/shopping/lists/"+url.PathEscape(listID)+"/recipe", nil,
		[]map[string]string{{"recipeId": r.ID}}, nil)
}

// CheckOff marks an item bought: it stays on the list checked, as in the app.
func (c *Client) CheckOff(ctx context.Context, cb dom.Cookbook, itemID string) error {
	// The update takes the whole item, a missing field would be reset
	var item map[string]any
	path := "/api/households/shopping/items/" + url.PathEscape(itemID)
	if err := c.call(ctx, cb, http.MethodGet, path, nil, nil, &item); err != nil {
		return err
	}
	item["checked"] = true
	return c.call(ctx, cb, http.MethodPut, path, nil, item, nil)
}

func (c *Client) RemoveItem(ctx context.Context, cb dom.Cookbook, itemID string) error {
	return c.call(ctx, cb, http.MethodDelete, "/api/households/shopping/items/"+url.PathEscape(itemID), nil, nil, nil)
}

func nonNil(lines []string) []string {
	if lines == nil {
		return []string{}
	}
	return lines
}
