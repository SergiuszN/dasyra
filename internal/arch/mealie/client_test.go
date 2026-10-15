package mealie

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dasyra/internal/dom"
)

var ctx = context.Background()

// server answers as Mealie does and records what it was sent.
func server(t *testing.T, got map[string]string) dom.Cookbook {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		got[r.Method+" "+r.URL.Path] = string(body)
		switch r.Method + " " + r.URL.Path {
		case "POST /api/recipes/create/html-or-json":
			io.WriteString(w, `"blini"`)
		case "PATCH /api/recipes/blini":
			io.WriteString(w, `{"slug":"blini"}`)
		case "GET /api/users/self":
			io.WriteString(w, `{"username":"alex","groupSlug":"home","household":"Family"}`)
		case "GET /api/recipes/blini":
			io.WriteString(w, `{"id":"r1","slug":"blini","name":"Блины","recipeYield":"","recipeServings":4,"totalTime":"40 мин",
				"tags":[{"name":"Завтрак"}],"recipeIngredient":[{"display":"","note":"2 яйца"},{"display":"500 ml milk","note":"","title":"Тесто"}],
				"recipeInstructions":[{"title":"","text":"Смешать"}],"notes":[]}`)
		case "GET /api/households/shopping/lists/l1":
			io.WriteString(w, `{"id":"l1","name":"Продукты","listItems":[
				{"id":"i1","note":"молоко 2 л","display":"1 молоко 2 л","food":null,"checked":false},
				{"id":"i2","note":"","display":"2 kg potatoes","food":{"name":"potatoes"},"checked":true}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"detail":"Not found"}`)
		}
	}))
	t.Cleanup(s.Close)
	return dom.Cookbook{URL: s.URL, PublicURL: "https://recipes.example/", Token: "tok"}
}

func TestCreateSendsJSONLDAndSetsTheTimeApart(t *testing.T) {
	got := map[string]string{}
	c := server(t, got)
	slug, err := New().Create(ctx, c, dom.Recipe{Name: "Блины", Ingredients: []string{"2 яйца"}, Steps: []string{"Смешать"}, TotalTime: "40 мин", Tags: []string{"завтрак", "быстро"}})
	if err != nil || slug != "blini" {
		t.Fatal(slug, err)
	}
	var body struct {
		Data        string `json:"data"`
		IncludeTags bool   `json:"includeTags"`
	}
	json.Unmarshal([]byte(got["POST /api/recipes/create/html-or-json"]), &body)
	if !body.IncludeTags || !strings.Contains(body.Data, `"recipeIngredient":["2 яйца"]`) || !strings.Contains(body.Data, `"keywords":"завтрак, быстро"`) ||
		!strings.Contains(body.Data, `"@type":"HowToStep","text":"Смешать"`) {
		t.Fatalf("JSON-LD: %s", body.Data)
	}
	if got["PATCH /api/recipes/blini"] != `{"totalTime":"40 мин"}` {
		t.Fatalf("time: %s", got["PATCH /api/recipes/blini"])
	}
}

func TestRecipeReadsLinesAndLinksToTheGroup(t *testing.T) {
	c := server(t, map[string]string{})
	r, err := New().Recipe(ctx, c, "blini")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2 яйца", "## Тесто", "500 ml milk"}
	if strings.Join(r.Ingredients, "|") != strings.Join(want, "|") || r.Servings != "4" || r.Tags[0] != "Завтрак" {
		t.Fatalf("recipe: %+v", r)
	}
	if r.Link != "https://recipes.example/g/home/r/blini" {
		t.Fatalf("link: %s", r.Link)
	}
	if missing, err := New().Recipe(ctx, c, "nope"); missing != nil || err != nil {
		t.Fatal("a missing recipe is nil", err)
	}
	c.Token = "wrong"
	if _, err := New().Recipe(ctx, c, "blini"); err == nil || !strings.Contains(err.Error(), "refused the token") {
		t.Fatal(err)
	}
}

func TestShoppingListShowsPlainLinesAsWritten(t *testing.T) {
	c := server(t, map[string]string{})
	l, err := New().ShoppingList(ctx, c, "l1")
	if err != nil {
		t.Fatal(err)
	}
	if l.Items[0].Text != "молоко 2 л" || l.Items[1].Text != "2 kg potatoes" || !l.Items[1].Done {
		t.Fatalf("items: %+v", l.Items)
	}
}
