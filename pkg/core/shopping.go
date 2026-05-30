package core

import (
	"context"
	"fmt"
	"net/url"
)

// ShoppingList is a household shopping list. ListItems is populated by
// GetShoppingList but is typically empty in the list/summary view.
type ShoppingList struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	CreatedAt string             `json:"createdAt,omitempty"`
	UpdatedAt string             `json:"updatedAt,omitempty"`
	ListItems []ShoppingListItem `json:"listItems,omitempty"`
}

// ShoppingListItem is one entry on a shopping list. Field names mirror the
// Mealie ShoppingListItemOut schema.
type ShoppingListItem struct {
	ID             string    `json:"id"`
	ShoppingListID string    `json:"shoppingListId"`
	Checked        bool      `json:"checked"`
	Position       int       `json:"position"`
	Note           string    `json:"note,omitempty"`
	Quantity       *float64  `json:"quantity,omitempty"`
	FoodID         *string   `json:"foodId,omitempty"`
	Food           *NamedRef `json:"food,omitempty"`
	UnitID         *string   `json:"unitId,omitempty"`
	Unit           *NamedRef `json:"unit,omitempty"`
	LabelID        *string   `json:"labelId,omitempty"`
	Display        string    `json:"display,omitempty"`
}

// itemsCollection is the Mealie ShoppingListItemsCollectionOut envelope returned
// by the bulk-capable create (POST) and update (PUT) item endpoints.
type itemsCollection struct {
	CreatedItems []ShoppingListItem `json:"createdItems"`
	UpdatedItems []ShoppingListItem `json:"updatedItems"`
	DeletedItems []ShoppingListItem `json:"deletedItems"`
}

// CreateShoppingItem is the payload for adding a free-text item to a list. Only
// ShoppingListID is required by Mealie; a note-only item is created by sending
// just the note (and an optional quantity).
type CreateShoppingItem struct {
	ShoppingListID string   `json:"shoppingListId"`
	Note           string   `json:"note,omitempty"`
	Quantity       *float64 `json:"quantity,omitempty"`
}

// ListShoppingLists returns a page of shopping lists.
func (c *Client) ListShoppingLists(ctx context.Context, opts ListOptions) (*Page[ShoppingList], error) {
	q := url.Values{}
	opts.apply(q)
	var page Page[ShoppingList]
	if err := c.do(ctx, "GET", "/api/households/shopping/lists", q, nil, &page); err != nil {
		return nil, err
	}
	return &page, nil
}

// GetShoppingList fetches a single shopping list, including its items.
func (c *Client) GetShoppingList(ctx context.Context, id string) (*ShoppingList, error) {
	var list ShoppingList
	if err := c.do(ctx, "GET", "/api/households/shopping/lists/"+url.PathEscape(id), nil, nil, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// CreateShoppingList creates a new shopping list with the given name.
func (c *Client) CreateShoppingList(ctx context.Context, name string) (*ShoppingList, error) {
	var list ShoppingList
	if err := c.do(ctx, "POST", "/api/households/shopping/lists", nil, map[string]string{"name": name}, &list); err != nil {
		return nil, err
	}
	return &list, nil
}

// DeleteShoppingList deletes a shopping list by id.
func (c *Client) DeleteShoppingList(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/households/shopping/lists/"+url.PathEscape(id), nil, nil, nil)
}

// AddShoppingItem adds an item to a list. The create endpoint returns a
// ShoppingListItemsCollectionOut; the newly created item is returned.
func (c *Client) AddShoppingItem(ctx context.Context, in CreateShoppingItem) (*ShoppingListItem, error) {
	var coll itemsCollection
	if err := c.do(ctx, "POST", "/api/households/shopping/items", nil, in, &coll); err != nil {
		return nil, err
	}
	if len(coll.CreatedItems) == 0 {
		return nil, fmt.Errorf("item create returned no items")
	}
	return &coll.CreatedItems[0], nil
}

// GetShoppingItem fetches a single shopping list item by id.
func (c *Client) GetShoppingItem(ctx context.Context, id string) (*ShoppingListItem, error) {
	var item ShoppingListItem
	if err := c.do(ctx, "GET", "/api/households/shopping/items/"+url.PathEscape(id), nil, nil, &item); err != nil {
		return nil, err
	}
	return &item, nil
}

// SetItemChecked toggles the checked state of an item. It round-trips the full
// raw item (as a map) so fields outside the curated struct — labelId, foodId,
// unitId, extras, recipe references, position — are preserved on update, then
// decodes the ShoppingListItemsCollectionOut update envelope.
func (c *Client) SetItemChecked(ctx context.Context, id string, checked bool) (*ShoppingListItem, error) {
	path := "/api/households/shopping/items/" + url.PathEscape(id)
	var raw map[string]any
	if err := c.do(ctx, "GET", path, nil, nil, &raw); err != nil {
		return nil, err
	}
	raw["checked"] = checked
	var coll itemsCollection
	if err := c.do(ctx, "PUT", path, nil, raw, &coll); err != nil {
		return nil, err
	}
	if len(coll.UpdatedItems) == 0 {
		return nil, fmt.Errorf("item update returned no items")
	}
	return &coll.UpdatedItems[0], nil
}

// DeleteShoppingItem removes an item by id.
func (c *Client) DeleteShoppingItem(ctx context.Context, id string) error {
	return c.do(ctx, "DELETE", "/api/households/shopping/items/"+url.PathEscape(id), nil, nil, nil)
}
