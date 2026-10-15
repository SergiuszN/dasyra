package tool

import (
	"context"
	"fmt"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// FindRecipes searches the user's cookbook and reads or sends one recipe: like send_note, sending delivers it
// as saved, so the model never retypes amounts.
type FindRecipes struct {
	withCookbook
	API      CookbookAPI
	Messages dom.Messages
	Chat     Sender
	IDs      IDs
}

const FindRecipesName = "find_recipes"

func (FindRecipes) Spec() llm.Tool {
	return Spec(FindRecipesName, "Look into the user's cookbook (their recipe app). With query and/or tags lists matching recipes, without both lists them all by name. With slug reads that recipe in full, or with send=true sends it to the chat exactly as saved instead of retyping it (your reply comes after it, keep it to a line or two). Read a recipe in full to write an adapted version (half the portion, without nuts)",
		Str("query", "Words to search names, descriptions and ingredients by"),
		List("tags", "string", "Tag names the recipes must all have"),
		Str("slug", "Slug of a recipe to read or send"),
		Bool("send", "true to send the recipe with this slug to the chat as saved"),
		Int("offset", "Offset (default 0)"),
		Int("limit", "Max recipes (default 20)"),
	)
}

func (t FindRecipes) Run(ctx context.Context, s *Session, a Args) (string, error) {
	if slug := a.String("slug"); slug != "" {
		send, _ := a.Bool("send")
		return t.read(ctx, s, slug, send)
	}
	query, tags := a.String("query"), a.Strings("tags")
	offset, limit := a.IntOr("offset", 0), min(max(a.IntOr("limit", 20), 1), 50)
	recipes, err := t.API.Search(ctx, *s.Cookbook, query, tags, offset-offset%limit, limit)
	if err != nil {
		return cookbookError(err), nil
	}
	if len(recipes) == 0 {
		if query == "" && len(tags) == 0 && offset == 0 {
			return "The cookbook is empty.", nil
		}
		return "No recipes found.", nil
	}
	type card struct {
		Slug      string   `json:"slug"`
		Name      string   `json:"name"`
		Tags      []string `json:"tags,omitempty"`
		TotalTime string   `json:"total_time,omitempty"`
	}
	cards := make([]card, len(recipes))
	for i, r := range recipes {
		cards[i] = card{r.Slug, r.Name, r.Tags, r.TotalTime}
	}
	return toJSON(map[string]any{"count": len(cards), "recipes": cards, "next_offset": nextOffset(len(cards), offset-offset%limit, limit)}), nil
}

func (t FindRecipes) read(ctx context.Context, s *Session, slug string, send bool) (string, error) {
	r, err := t.API.Recipe(ctx, *s.Cookbook, slug)
	if err != nil {
		return cookbookError(err), nil
	}
	if r == nil {
		return fmt.Sprintf("Error: no recipe %q in the cookbook, search for it by words first.", slug), nil
	}
	s.Recipes[r.Slug] = true
	if !send {
		return toJSON(rowOf(*r)), nil
	}
	if err := t.Chat.Send(ctx, s.User.ChatID, recipeText(*r)); err != nil {
		return fmt.Sprintf("Error: Telegram did not take the recipe (%s), nothing was sent.", err), nil
	}
	sent := &dom.Message{ID: t.IDs.Next(), Role: dom.RoleAssistant, Content: fmt.Sprintf("[sent recipe %q (%s) from the cookbook]", r.Name, r.Slug), CreatedAt: time.Now()}
	if err := t.Messages.Insert(ctx, s.User.ID, sent); err != nil {
		return "", err
	}
	return fmt.Sprintf("Recipe %q was sent to the chat.", r.Name), nil
}
