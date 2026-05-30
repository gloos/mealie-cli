package core

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// RecipeSummary is the list/summary view of a recipe. Field names mirror the
// Mealie API (camelCase) so the curated set is a faithful, stable subset.
type RecipeSummary struct {
	ID          string         `json:"id,omitempty"`
	Slug        string         `json:"slug"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Image       string         `json:"image,omitempty"`
	RecipeYield string         `json:"recipeYield,omitempty"`
	TotalTime   string         `json:"totalTime,omitempty"`
	PrepTime    string         `json:"prepTime,omitempty"`
	CookTime    string         `json:"cookTime,omitempty"`
	Rating      *float64       `json:"rating,omitempty"`
	Categories  []OrganizerRef `json:"recipeCategory,omitempty"`
	Tags        []OrganizerRef `json:"tags,omitempty"`
	DateAdded   string         `json:"dateAdded,omitempty"`
	DateUpdated string         `json:"dateUpdated,omitempty"`
}

// RecipeIngredient is one ingredient line. Display/OriginalText carry the
// human-readable rendering; Food/Unit are the structured references when parsed.
type RecipeIngredient struct {
	Quantity     *float64  `json:"quantity,omitempty"`
	Unit         *NamedRef `json:"unit,omitempty"`
	Food         *NamedRef `json:"food,omitempty"`
	Note         string    `json:"note,omitempty"`
	Display      string    `json:"display,omitempty"`
	Title        string    `json:"title,omitempty"`
	OriginalText string    `json:"originalText,omitempty"`
}

// RecipeStep is one instruction step.
type RecipeStep struct {
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
}

// RecipeNote is a free-text note attached to a recipe.
type RecipeNote struct {
	Title string `json:"title,omitempty"`
	Text  string `json:"text,omitempty"`
}

// Recipe is the full recipe detail. It embeds RecipeSummary so summary fields
// appear inline in JSON output.
type Recipe struct {
	RecipeSummary
	RecipeIngredient   []RecipeIngredient `json:"recipeIngredient,omitempty"`
	RecipeInstructions []RecipeStep       `json:"recipeInstructions,omitempty"`
	Tools              []OrganizerRef     `json:"tools,omitempty"`
	Notes              []RecipeNote       `json:"notes,omitempty"`
	OrgURL             string             `json:"orgURL,omitempty"`
}

// RecipeListOptions extends ListOptions with recipe-specific filters that map to
// the convenience query parameters on GET /api/recipes.
type RecipeListOptions struct {
	ListOptions
	Categories []string
	Tags       []string
	Tools      []string
	Cookbook   string
}

// ListRecipes returns a page of recipe summaries.
func (c *Client) ListRecipes(ctx context.Context, opts RecipeListOptions) (*Page[RecipeSummary], error) {
	q := url.Values{}
	opts.ListOptions.apply(q)
	for _, v := range opts.Categories {
		q.Add("categories", v)
	}
	for _, v := range opts.Tags {
		q.Add("tags", v)
	}
	for _, v := range opts.Tools {
		q.Add("tools", v)
	}
	if opts.Cookbook != "" {
		q.Set("cookbook", opts.Cookbook)
	}
	var page Page[RecipeSummary]
	if err := c.do(ctx, "GET", "/api/recipes", q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetRecipe fetches a single recipe by slug.
func (c *Client) GetRecipe(ctx context.Context, slug string) (*Recipe, error) {
	var r Recipe
	if err := c.do(ctx, "GET", "/api/recipes/"+url.PathEscape(slug), nil, nil, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CreateRecipe creates an empty recipe with the given name and returns its slug.
func (c *Client) CreateRecipe(ctx context.Context, name string) (string, error) {
	var slug string
	if err := c.do(ctx, "POST", "/api/recipes", nil, map[string]string{"name": name}, &slug); err != nil {
		return "", err
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", fmt.Errorf("server returned an empty recipe slug")
	}
	return slug, nil
}

// CreateRecipeFromURL scrapes the given URL into a new recipe and returns its slug.
func (c *Client) CreateRecipeFromURL(ctx context.Context, recipeURL string, includeTags bool) (string, error) {
	body := map[string]any{"url": recipeURL, "includeTags": includeTags}
	var slug string
	if err := c.do(ctx, "POST", "/api/recipes/create/url", nil, body, &slug); err != nil {
		return "", err
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", fmt.Errorf("server returned an empty recipe slug")
	}
	return slug, nil
}

// DeleteRecipe deletes a recipe by slug.
func (c *Client) DeleteRecipe(ctx context.Context, slug string) error {
	return c.do(ctx, "DELETE", "/api/recipes/"+url.PathEscape(slug), nil, nil, nil)
}
