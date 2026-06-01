package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gloos/mealie-cli/pkg/core"
)

func ptrFloat(f float64) *float64 { return &f }

// TestRenderRecipeGolden pins the human rendering of a recipe so a regression in
// the table view surfaces as a reviewable diff rather than shipping silently.
func TestRenderRecipeGolden(t *testing.T) {
	full := &core.Recipe{
		RecipeSummary: core.RecipeSummary{
			Name:        "Chana Masala",
			Description: "A hearty chickpea curry.",
			RecipeYield: "4 servings",
			TotalTime:   "PT45M",
			Tags:        []core.OrganizerRef{{Name: "vegan"}, {Name: "curry"}},
		},
		RecipeIngredient: []core.RecipeIngredient{
			{Title: "For the base", Display: "2 onions, diced"},
			{Display: "1 tin chickpeas"},
			{OriginalText: "2 tsp garam masala"},
		},
		RecipeInstructions: []core.RecipeStep{
			{Title: "Prep", Text: "Dice the onions."},
			{Text: "Simmer for 30 minutes."},
		},
	}
	var buf bytes.Buffer
	renderRecipe(&buf, full)
	assertGolden(t, "recipe_full.txt", buf.String())

	minimal := &core.Recipe{RecipeSummary: core.RecipeSummary{Name: "Toast"}}
	buf.Reset()
	renderRecipe(&buf, minimal)
	assertGolden(t, "recipe_minimal.txt", buf.String())
}

func TestShoppingListDetailGolden(t *testing.T) {
	populated := &core.ShoppingList{
		ID:   "l1",
		Name: "Weekly",
		ListItems: []core.ShoppingListItem{
			{ID: "i1", Checked: false, Quantity: ptrFloat(2), Display: "2 onions"},
			{ID: "i2", Checked: true, Note: "Eggs"},
			{ID: "i3", Note: "Salt"},
		},
	}
	var buf bytes.Buffer
	shoppingListDetail(&buf, populated)
	assertGolden(t, "shopping_detail.txt", buf.String())

	empty := &core.ShoppingList{ID: "l2", Name: "Empty"}
	buf.Reset()
	shoppingListDetail(&buf, empty)
	assertGolden(t, "shopping_empty.txt", buf.String())
}

func TestMealplanTableGolden(t *testing.T) {
	items := []core.MealPlan{
		{ID: 1, Date: "2026-06-01", EntryType: "dinner", Title: "Curry night"},
		{ID: 2, Date: "2026-06-02", EntryType: "lunch", Recipe: &core.RecipeSummary{Name: "Soup"}},
		{ID: 3, Date: "2026-06-03", EntryType: "snack", Text: "Leftovers"},
		{ID: 4, Date: "2026-06-04", EntryType: "breakfast"},
	}
	var buf bytes.Buffer
	if err := mealplanTable(&buf, items); err != nil {
		t.Fatalf("mealplanTable: %v", err)
	}
	assertGolden(t, "mealplan_table.txt", buf.String())
}

func TestOrgNames(t *testing.T) {
	cases := []struct {
		refs []core.OrganizerRef
		want string
	}{
		{nil, ""},
		{[]core.OrganizerRef{{Name: "vegan"}}, "vegan"},
		{[]core.OrganizerRef{{Name: "vegan"}, {Name: "curry"}}, "vegan, curry"},
	}
	for _, c := range cases {
		if got := orgNames(c.refs); got != c.want {
			t.Errorf("orgNames(%v) = %q, want %q", c.refs, got, c.want)
		}
	}
}

func TestDash(t *testing.T) {
	if got := dash(""); got != "-" {
		t.Errorf("dash(\"\") = %q, want -", got)
	}
	if got := dash("x"); got != "x" {
		t.Errorf("dash(\"x\") = %q, want x", got)
	}
}

func TestNewTable(t *testing.T) {
	var buf bytes.Buffer
	tw := newTable(&buf, "A", "B")
	tw.Flush()
	got := buf.String()
	if !strings.Contains(got, "A") || !strings.Contains(got, "B") {
		t.Errorf("newTable header missing, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n") {
		t.Errorf("header row should end in a newline, got %q", got)
	}
}
