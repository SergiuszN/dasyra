package tool

import (
	"context"
	"fmt"
	"strings"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// ShoppingList is a shopping list of the household in the cookbook app, shared with whoever is in it.
type ShoppingList struct {
	withCookbook
	API CookbookAPI
}

// shortID is how the model refers to an item: the server's ids are long UUIDs
const shortID = 8

func (ShoppingList) Spec() llm.Tool {
	return Spec("shopping_list", "A shopping list in the user's cookbook app, shared with their household. Shows what is left to buy, after the changes asked: add lines or every ingredient of a recipe, tick items as bought, remove wrong ones",
		Str("list", "List name (default the first list); a new name creates the list when adding"),
		List("add", "string", "Lines to add as the user said them, e.g. \"milk 2 l\""),
		Str("add_recipe", "Slug of a recipe whose ingredients to add"),
		List("bought", "string", "Ids of items bought"),
		List("remove", "string", "Ids of items to remove"),
	)
}

func (t ShoppingList) Run(ctx context.Context, s *Session, a Args) (string, error) {
	c, name := *s.Cookbook, a.String("list")
	add, recipe, bought, remove := a.Strings("add"), a.String("add_recipe"), a.Strings("bought"), a.Strings("remove")
	lists, err := t.API.ShoppingLists(ctx, c)
	if err != nil {
		return cookbookError(err), nil
	}
	target := pickList(lists, name)
	if target == nil {
		switch {
		case name != "" && (len(add) > 0 || recipe != ""):
			created, err := t.API.CreateList(ctx, c, name)
			if err != nil {
				return cookbookError(err), nil
			}
			target, lists = &created, append(lists, created)
		case name != "":
			return fmt.Sprintf("Error: no list %q. Lists: %s.", name, listNames(lists)), nil
		default:
			return "No shopping lists yet: pass a list name in the user's language to create one when adding.", nil
		}
	}

	var problems []string
	if len(bought) > 0 || len(remove) > 0 {
		current, err := t.API.ShoppingList(ctx, c, target.ID)
		if err != nil {
			return cookbookError(err), nil
		}
		for _, id := range bought {
			if problem := t.change(ctx, c, current, id, t.API.CheckOff); problem != "" {
				problems = append(problems, problem)
			}
		}
		for _, id := range remove {
			if problem := t.change(ctx, c, current, id, t.API.RemoveItem); problem != "" {
				problems = append(problems, problem)
			}
		}
	}
	if len(add) > 0 {
		if err := t.API.AddToList(ctx, c, target.ID, add); err != nil {
			problems = append(problems, "adding: "+err.Error())
		}
	}
	if recipe != "" {
		if err := t.API.AddRecipeToList(ctx, c, target.ID, recipe); err != nil {
			problems = append(problems, "adding the recipe: "+err.Error())
		}
	}

	current, err := t.API.ShoppingList(ctx, c, target.ID)
	if err != nil {
		return cookbookError(err), nil
	}
	type row struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}
	open, done := []row{}, 0
	for _, item := range current.Items {
		if item.Done {
			done++
			continue
		}
		open = append(open, row{item.ID[:min(len(item.ID), shortID)], item.Text})
	}
	view := map[string]any{"list": current.Name, "to_buy": open, "bought": done}
	if others := otherLists(lists, target.ID); len(others) > 0 {
		view["other_lists"] = others
	}
	result := toJSON(view)
	if len(problems) > 0 {
		result = "Errors: " + strings.Join(problems, "; ") + "\n" + result
	}
	return result, nil
}

// change finds the item by the start of its id and applies do to it.
func (t ShoppingList) change(ctx context.Context, c dom.Cookbook, l *dom.ShoppingList, id string, do func(context.Context, dom.Cookbook, string) error) string {
	id = strings.TrimSpace(id)
	for _, item := range l.Items {
		if id != "" && strings.HasPrefix(item.ID, id) {
			if err := do(ctx, c, item.ID); err != nil {
				return fmt.Sprintf("item %s: %s", id, err)
			}
			return ""
		}
	}
	return fmt.Sprintf("no item %s on the list", id)
}

func pickList(lists []dom.ShoppingList, name string) *dom.ShoppingList {
	if len(lists) == 0 {
		return nil
	}
	if name == "" {
		return &lists[0]
	}
	for i := range lists {
		if strings.EqualFold(strings.TrimSpace(lists[i].Name), strings.TrimSpace(name)) {
			return &lists[i]
		}
	}
	return nil
}

func otherLists(lists []dom.ShoppingList, except string) []string {
	var names []string
	for _, l := range lists {
		if l.ID != except {
			names = append(names, l.Name)
		}
	}
	return names
}

func listNames(lists []dom.ShoppingList) string {
	if len(lists) == 0 {
		return "none"
	}
	return strings.Join(otherLists(lists, ""), ", ")
}
