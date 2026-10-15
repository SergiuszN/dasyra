package dom

// Cookbook is the user's own recipe server (Mealie) with their API token, connected in the admin panel.
// Recipes, the meal plan and the shopping list live there, so the user sees them in its app too.
type Cookbook struct {
	UserID    int64
	URL       string // where the API is called, e.g. http://mealie:9000
	PublicURL string // where the user opens it, for links to recipes; empty means no links
	Token     string
}

func (c Cookbook) MaskedToken() string { return Mask(c.Token) }

// Mask shows only the end of a secret, the panel never prints one in full.
func Mask(secret string) string {
	if secret == "" {
		return "none"
	}
	if len(secret) <= 8 {
		return "••••"
	}
	return "••••" + secret[len(secret)-4:]
}

// Recipe is a recipe in the cookbook. Ingredients and Steps are plain lines, the server keeps its own structure.
type Recipe struct {
	ID          string
	Slug        string
	Name        string
	Description string
	Servings    string
	TotalTime   string
	Ingredients []string
	Steps       []string
	Tags        []string
	Notes       []string
	Source      string // the page it came from
	Link        string // where the user opens it, empty without a public URL
}

// Meals of a meal plan entry, as Mealie names them.
var Meals = []string{"breakfast", "lunch", "dinner", "side", "snack", "drink", "dessert"}

// PlanEntry is one meal on a day of the meal plan: a recipe or just a title.
type PlanEntry struct {
	ID         int64
	Day        string // Y-m-d
	Meal       string
	Title      string
	RecipeSlug string
	RecipeName string
}

// ShoppingItem is a line of a shopping list.
type ShoppingItem struct {
	ID   string
	Text string
	Done bool
}

type ShoppingList struct {
	ID    string
	Name  string
	Items []ShoppingItem
}
