package tool_test

import (
	"context"
	"strings"
	"testing"

	"dasyra/internal/app/tool"
	"dasyra/internal/arch/sqlite"
	"dasyra/internal/dom"
)

// kitchen is a cookbook server in memory.
type kitchen struct {
	recipes map[string]dom.Recipe
	plan    []dom.PlanEntry
	lists   []dom.ShoppingList
	updates []dom.Recipe
	imports []string
}

func newKitchen() *kitchen {
	return &kitchen{recipes: map[string]dom.Recipe{
		"borshch": {Slug: "borshch", Name: "Борщ", Ingredients: []string{"500 г говядины", "1 свёкла"}, Steps: []string{"Сварить бульон"}, Link: "https://recipes.example/g/home/r/borshch"},
	}}
}

func (k *kitchen) Search(_ context.Context, _ dom.Cookbook, query string, _ []string, _, _ int) ([]dom.Recipe, error) {
	var out []dom.Recipe
	for _, r := range k.recipes {
		if strings.Contains(strings.ToLower(r.Name), strings.ToLower(query)) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (k *kitchen) Recipe(_ context.Context, _ dom.Cookbook, slug string) (*dom.Recipe, error) {
	r, ok := k.recipes[slug]
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (k *kitchen) Import(_ context.Context, _ dom.Cookbook, url string) (string, error) {
	k.imports = append(k.imports, url)
	k.recipes["lasagna"] = dom.Recipe{Slug: "lasagna", Name: "Lasagna", Ingredients: []string{"pasta"}}
	return "lasagna", nil
}

func (k *kitchen) Create(_ context.Context, _ dom.Cookbook, r dom.Recipe) (string, error) {
	r.Slug = strings.ToLower(r.Name)
	k.recipes[r.Slug] = r
	return r.Slug, nil
}

func (k *kitchen) Update(_ context.Context, _ dom.Cookbook, slug string, r dom.Recipe) (string, error) {
	k.updates = append(k.updates, r)
	return slug, nil
}

func (k *kitchen) Plan(_ context.Context, _ dom.Cookbook, from, to string) ([]dom.PlanEntry, error) {
	var out []dom.PlanEntry
	for _, e := range k.plan {
		if e.Day >= from && e.Day <= to {
			out = append(out, e)
		}
	}
	return out, nil
}

func (k *kitchen) AddToPlan(_ context.Context, _ dom.Cookbook, e dom.PlanEntry) error {
	e.ID = int64(len(k.plan) + 1)
	k.plan = append(k.plan, e)
	return nil
}

func (k *kitchen) RemoveFromPlan(context.Context, dom.Cookbook, int64) error { return nil }

func (k *kitchen) ShoppingLists(context.Context, dom.Cookbook) ([]dom.ShoppingList, error) {
	out := make([]dom.ShoppingList, len(k.lists))
	for i, l := range k.lists {
		out[i] = dom.ShoppingList{ID: l.ID, Name: l.Name}
	}
	return out, nil
}

func (k *kitchen) CreateList(_ context.Context, _ dom.Cookbook, name string) (dom.ShoppingList, error) {
	l := dom.ShoppingList{ID: "list-" + name, Name: name}
	k.lists = append(k.lists, l)
	return l, nil
}

func (k *kitchen) list(id string) *dom.ShoppingList {
	for i := range k.lists {
		if k.lists[i].ID == id {
			return &k.lists[i]
		}
	}
	return nil
}

func (k *kitchen) ShoppingList(_ context.Context, _ dom.Cookbook, id string) (*dom.ShoppingList, error) {
	l := *k.list(id)
	l.Items = append([]dom.ShoppingItem(nil), l.Items...)
	return &l, nil
}

func (k *kitchen) AddToList(_ context.Context, _ dom.Cookbook, listID string, lines []string) error {
	l := k.list(listID)
	for _, line := range lines {
		l.Items = append(l.Items, dom.ShoppingItem{ID: strings.Repeat(string(rune('a'+len(l.Items))), 8) + "-4b2c-uuid", Text: line})
	}
	return nil
}

func (k *kitchen) AddRecipeToList(ctx context.Context, c dom.Cookbook, listID, slug string) error {
	return k.AddToList(ctx, c, listID, k.recipes[slug].Ingredients)
}

func (k *kitchen) CheckOff(_ context.Context, _ dom.Cookbook, itemID string) error {
	for _, l := range k.lists {
		for i := range l.Items {
			if l.Items[i].ID == itemID {
				l.Items[i].Done = true
			}
		}
	}
	return nil
}

func (k *kitchen) RemoveItem(context.Context, dom.Cookbook, string) error { return nil }

type cookbookEnv struct {
	*env
	kitchen *kitchen
	chat    *sent
}

func newCookbookEnv(t *testing.T) *cookbookEnv {
	e := &cookbookEnv{env: newEnv(t), kitchen: newKitchen(), chat: &sent{}}
	e.registry = tool.NewRegistry(discard,
		tool.CreateMemory{Memories: e.memories, IDs: &sqlite.IDs{}},
		tool.FindRecipes{API: e.kitchen, Messages: sqlite.Messages{DB: e.db}, Chat: e.chat, IDs: &sqlite.IDs{}},
		tool.SaveRecipe{API: e.kitchen}, tool.MealPlan{API: e.kitchen}, tool.ShoppingList{API: e.kitchen},
	)
	e.session.Cookbook = &dom.Cookbook{UserID: 1, URL: "http://mealie:9000", Token: "t"}
	return e
}

func TestCookbookToolsOnlyForUsersWithACookbook(t *testing.T) {
	e := newCookbookEnv(t)
	if specs := e.registry.Specs(e.session); len(specs) != 5 {
		t.Fatalf("with a cookbook: %d tools", len(specs))
	}
	e.session.Cookbook = nil
	if specs := e.registry.Specs(e.session); len(specs) != 1 || specs[0].Name != "create_memory" {
		t.Fatalf("without a cookbook: %v", specs)
	}
	if got := e.run("find_recipes", `{"query": "борщ"}`); got != "Unknown tool: find_recipes" {
		t.Fatalf("a hidden tool must not run: %s", got)
	}
}

func TestFindRecipesSendsAsSavedAndUnlocksChanges(t *testing.T) {
	e := newCookbookEnv(t)
	if got := e.run("find_recipes", `{"query": "борщ"}`); !strings.Contains(got, `"slug":"borshch"`) || strings.Contains(got, "говядины") {
		t.Fatalf("a search lists cards only: %s", got)
	}
	if got := e.run("save_recipe", `{"slug": "borshch", "servings": "4"}`); !strings.Contains(got, "read recipe") {
		t.Fatalf("changing an unread recipe: %s", got)
	}
	if got := e.run("find_recipes", `{"slug": "borshch", "send": true}`); !strings.Contains(got, "was sent") {
		t.Fatal(got)
	}
	if text := e.chat.texts[0]; !strings.Contains(text, "• 500 г говядины") || !strings.Contains(text, "1. Сварить бульон") || !strings.HasSuffix(text, "/r/borshch") {
		t.Fatalf("sent recipe:\n%s", text)
	}
	if got := e.run("save_recipe", `{"slug": "borshch", "servings": "4", "ingredients": ""}`); !strings.HasPrefix(got, "Saved") {
		t.Fatal(got)
	}
	if u := e.kitchen.updates[0]; u.Servings != "4" || u.Ingredients != nil {
		t.Fatalf("only the fields given change: %+v", u)
	}
}

func TestSaveRecipeImportsOnlyPublicPages(t *testing.T) {
	e := newCookbookEnv(t)
	for _, url := range []string{"http://searxng:8080/", "http://192.168.1.5/r", "file:///etc/passwd", "http://mealie.local/x"} {
		if got := e.run("save_recipe", `{"url": "`+url+`"}`); !strings.Contains(got, "only public") {
			t.Fatalf("%s: %s", url, got)
		}
	}
	if got := e.run("save_recipe", `{"url": "https://www.allrecipes.com/recipe/24074/lasagna/"}`); !strings.Contains(got, `Imported: {"slug":"lasagna"`) {
		t.Fatal(got)
	}
	if got := e.run("save_recipe", `{"name": "Блины"}`); !strings.Contains(got, "needs a name with ingredients") {
		t.Fatal(got)
	}
}

func TestMealPlanAddsAndWidensTheRange(t *testing.T) {
	e := newCookbookEnv(t)
	got := e.run("meal_plan", `{"from": "2026-10-16", "to": "2026-10-16", "add": [{"day": "2026-10-20", "slug": "borshch"}, {"day": "2026-10-16", "meal": "lunch", "title": "Салат"}, {"day": "2026-10-17"}]}`)
	if !strings.Contains(got, `"to":"2026-10-20"`) || !strings.Contains(got, `"meal":"dinner"`) || !strings.Contains(got, "Салат") {
		t.Fatalf("plan: %s", got)
	}
	if !strings.HasPrefix(got, "Errors: adding 2026-10-17 : a recipe slug or a title is required") {
		t.Fatalf("a bad entry is reported: %s", got)
	}
}

func TestShoppingListCreatesNamedListAndTicksByShortID(t *testing.T) {
	e := newCookbookEnv(t)
	if got := e.run("shopping_list", `{"add": ["молоко"]}`); !strings.Contains(got, "No shopping lists yet") {
		t.Fatal(got)
	}
	got := e.run("shopping_list", `{"list": "Продукты", "add": ["молоко 2 л"], "add_recipe": "borshch"}`)
	if !strings.Contains(got, `{"id":"aaaaaaaa","text":"молоко 2 л"}`) || !strings.Contains(got, "1 свёкла") {
		t.Fatalf("list: %s", got)
	}
	got = e.run("shopping_list", `{"bought": ["aaaaaaaa", "zzz"]}`)
	if strings.Contains(got, "молоко") || !strings.Contains(got, `"bought":1`) || !strings.Contains(got, "no item zzz") {
		t.Fatalf("after ticking: %s", got)
	}
}
