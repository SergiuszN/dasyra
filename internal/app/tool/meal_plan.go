package tool

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"dasyra/internal/app/llm"
	"dasyra/internal/dom"
)

// MealPlan is the household's meal plan in the cookbook: what is cooked on which day.
type MealPlan struct {
	withCookbook
	API CookbookAPI
}

func (MealPlan) Spec() llm.Tool {
	return Spec("meal_plan", "The meal plan in the user's cookbook: what they cook on which day. Lists the days from-to (default the next 7), after adding and removing entries if asked",
		Str("from", "First day to list, Y-m-d (default today)"),
		Str("to", "Last day to list, Y-m-d (default 6 days after from)"),
		Objects("add", "Meals to plan",
			Str("day", "Day, Y-m-d").Required(),
			Enum("meal", "Meal (default dinner)", dom.Meals...),
			Str("slug", "Slug of a cookbook recipe"),
			Str("title", "Or just a title, for a dish without a recipe"),
		),
		List("remove", "integer", "Ids of entries to remove"),
	)
}

func (t MealPlan) Run(ctx context.Context, s *Session, a Args) (string, error) {
	c, u := *s.Cookbook, s.User
	from, err := u.ParseDay(a.String("from"))
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if from == "" {
		from = u.Local(time.Now()).Format(dom.DayLayout)
	}
	to, err := u.ParseDay(a.String("to"))
	if err != nil {
		return "Error: " + err.Error(), nil
	}
	if to == "" {
		start, _ := time.Parse(dom.DayLayout, from)
		to = start.AddDate(0, 0, 6).Format(dom.DayLayout)
	}

	var problems []string
	for _, id := range a.Ints("remove") {
		if err := t.API.RemoveFromPlan(ctx, c, id); err != nil {
			problems = append(problems, fmt.Sprintf("entry %d: %s", id, err))
		}
	}
	for _, item := range a.Objects("add") {
		e, problem := t.entry(u, item)
		if problem == "" {
			if err := t.API.AddToPlan(ctx, c, e); err != nil {
				problem = err.Error()
			}
		}
		if problem != "" {
			problems = append(problems, fmt.Sprintf("adding %s %s: %s", item.String("day"), item.String("title")+item.String("slug"), problem))
			continue
		}
		from, to = min(from, e.Day), max(to, e.Day)
	}

	plan, err := t.API.Plan(ctx, c, from, to)
	if err != nil {
		return cookbookError(err), nil
	}
	type row struct {
		ID    int64  `json:"id"`
		Day   string `json:"day"`
		Meal  string `json:"meal"`
		Title string `json:"title"`
		Slug  string `json:"slug,omitempty"`
	}
	rows := []row{}
	for _, e := range plan {
		title := e.Title
		if e.RecipeName != "" {
			title = e.RecipeName
		}
		rows = append(rows, row{e.ID, e.Day, e.Meal, title, e.RecipeSlug})
	}
	result := toJSON(map[string]any{"from": from, "to": to, "plan": rows})
	if len(problems) > 0 {
		result = "Errors: " + strings.Join(problems, "; ") + "\n" + result
	}
	return result, nil
}

func (t MealPlan) entry(u *dom.User, a Args) (dom.PlanEntry, string) {
	day, err := u.ParseDay(a.String("day"))
	if err != nil || day == "" {
		return dom.PlanEntry{}, "a day Y-m-d is required"
	}
	e := dom.PlanEntry{Day: day, Meal: a.String("meal"), RecipeSlug: a.String("slug"), Title: a.String("title")}
	if e.Meal == "" {
		e.Meal = "dinner"
	}
	if !slices.Contains(dom.Meals, e.Meal) {
		return e, "unknown meal " + e.Meal
	}
	if e.RecipeSlug == "" && e.Title == "" {
		return e, "a recipe slug or a title is required"
	}
	return e, ""
}
