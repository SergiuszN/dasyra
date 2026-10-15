package tool

import (
	"context"
	"fmt"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// SaveRecipe puts a recipe into the user's cookbook: from a page the cookbook reads itself, or written by the model.
type SaveRecipe struct {
	withCookbook
	API CookbookAPI
}

func (SaveRecipe) Spec() llm.Tool {
	return Spec("save_recipe", "Save a recipe to the user's cookbook. With url the cookbook imports the recipe page itself (best for links). Otherwise write it: name with ingredients and steps, one line each, amounts as the user gave them. With slug changes a recipe you read in full in this reply: pass only the fields that change, a list replaces the old one whole",
		Str("url", "Recipe page to import"),
		Str("slug", "Slug of a recipe to change"),
		Str("name", "Recipe name"),
		List("ingredients", "string", "Ingredient lines with amounts, e.g. \"500 g beef\""),
		List("steps", "string", "Steps in order"),
		Str("servings", "Yield, e.g. \"4 servings\""),
		Str("total_time", "Total time, e.g. \"1 h 30 min\""),
		Str("description", "A sentence about the dish, only if the user or the source gave one"),
		List("tags", "string", "Tags, e.g. soup, quick dinner"),
		Str("source", "Where the recipe comes from, a URL"),
	)
}

func (t SaveRecipe) Run(ctx context.Context, s *Session, a Args) (string, error) {
	c := *s.Cookbook
	r := dom.Recipe{Name: a.String("name"), Description: a.String("description"), Servings: a.String("servings"),
		TotalTime: a.String("total_time"), Ingredients: a.Strings("ingredients"), Steps: a.Strings("steps"),
		Tags: a.Strings("tags"), Source: a.String("source")}
	switch url, slug := a.String("url"), a.String("slug"); {
	case url != "":
		if !publicPage(url) {
			return "Error: only public http(s) pages can be imported.", nil
		}
		slug, err := t.API.Import(ctx, c, url)
		if err != nil {
			return cookbookError(err) + " Read the page with web_fetch and save the recipe written out instead.", nil
		}
		return t.saved(ctx, s, slug, "Imported")
	case slug != "":
		if !s.Recipes[slug] {
			return fmt.Sprintf("Error: read recipe %q in full with %s first, then pass the fields that change.", slug, FindRecipesName), nil
		}
		newSlug, err := t.API.Update(ctx, c, slug, r)
		if err != nil {
			return cookbookError(err), nil
		}
		return t.saved(ctx, s, newSlug, "Saved")
	}
	if r.Name == "" || len(r.Ingredients) == 0 && len(r.Steps) == 0 {
		return "Error: a new recipe needs a name with ingredients or steps, or a url to import.", nil
	}
	slug, err := t.API.Create(ctx, c, r)
	if err != nil {
		return cookbookError(err), nil
	}
	return t.saved(ctx, s, slug, "Saved")
}

// saved shows what the cookbook holds now: an import may have read the page differently than expected.
func (t SaveRecipe) saved(ctx context.Context, s *Session, slug, what string) (string, error) {
	s.Recipes[slug] = true
	r, err := t.API.Recipe(ctx, *s.Cookbook, slug)
	if err != nil || r == nil {
		return fmt.Sprintf("%s as %q.", what, slug), nil
	}
	type summary struct {
		Slug        string   `json:"slug"`
		Name        string   `json:"name"`
		Ingredients int      `json:"ingredients"`
		Steps       int      `json:"steps"`
		Tags        []string `json:"tags,omitempty"`
		Link        string   `json:"link,omitempty"`
	}
	return what + ": " + toJSON(summary{r.Slug, r.Name, len(r.Ingredients), len(r.Steps), r.Tags, r.Link}), nil
}
