package core

import (
	"context"
	"net/url"
	"strconv"
)

// Meal plan entry types accepted by the API (the PlanEntryType enum).
const (
	EntryBreakfast = "breakfast"
	EntryLunch     = "lunch"
	EntryDinner    = "dinner"
	EntrySide      = "side"
	EntrySnack     = "snack"
	EntryDrink     = "drink"
	EntryDessert   = "dessert"
)

// EntryTypes lists the valid meal plan entry types, in display order.
var EntryTypes = []string{EntryBreakfast, EntryLunch, EntryDinner, EntrySide, EntrySnack, EntryDrink, EntryDessert}

// MealPlan is a single meal plan entry from /api/households/mealplans. Entry ids
// are integers in Mealie (unlike most resources which use UUID strings).
type MealPlan struct {
	ID        int            `json:"id"`
	Date      string         `json:"date"`
	EntryType string         `json:"entryType"`
	Title     string         `json:"title,omitempty"`
	Text      string         `json:"text,omitempty"`
	RecipeID  *string        `json:"recipeId,omitempty"`
	Recipe    *RecipeSummary `json:"recipe,omitempty"`
}

// CreateMealPlan is the payload for adding a meal plan entry. Provide either a
// RecipeID (to plan an existing recipe) or a Title/Text (for a free-text entry).
type CreateMealPlan struct {
	Date      string `json:"date"`
	EntryType string `json:"entryType"`
	Title     string `json:"title,omitempty"`
	Text      string `json:"text,omitempty"`
	RecipeID  string `json:"recipeId,omitempty"`
}

// ListMealPlans returns a page of meal plan entries, optionally restricted to a
// date range (inclusive, YYYY-MM-DD). Empty start/end disable that bound.
func (c *Client) ListMealPlans(ctx context.Context, opts ListOptions, start, end string) (*Page[MealPlan], error) {
	q := url.Values{}
	opts.apply(q)
	if start != "" {
		q.Set("start_date", start)
	}
	if end != "" {
		q.Set("end_date", end)
	}
	var page Page[MealPlan]
	if err := c.do(ctx, "GET", "/api/households/mealplans", q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// TodayMealPlans returns the meal plan entries scheduled for the current day.
func (c *Client) TodayMealPlans(ctx context.Context) ([]MealPlan, error) {
	var entries []MealPlan
	if err := c.do(ctx, "GET", "/api/households/mealplans/today", nil, nil, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

// CreateMealPlan adds a meal plan entry.
func (c *Client) CreateMealPlan(ctx context.Context, in CreateMealPlan) (*MealPlan, error) {
	var entry MealPlan
	if err := c.do(ctx, "POST", "/api/households/mealplans", nil, in, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

// DeleteMealPlan removes a meal plan entry by id.
func (c *Client) DeleteMealPlan(ctx context.Context, id int) error {
	return c.do(ctx, "DELETE", "/api/households/mealplans/"+strconv.Itoa(id), nil, nil, nil)
}
