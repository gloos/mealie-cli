package core

import (
	"net/url"
	"strconv"
)

// Page is the normalised pagination envelope. Upstream Mealie uses snake_case
// keys here (per_page/total_pages) even though list query parameters are
// camelCase; this type pins the response contract regardless.
type Page[T any] struct {
	Page       int     `json:"page"`
	PerPage    int     `json:"per_page"`
	Total      int     `json:"total"`
	TotalPages int     `json:"total_pages"`
	Items      []T     `json:"items"`
	Next       *string `json:"next"`
	Previous   *string `json:"previous"`
}

// OrganizerRef is a category, tag or tool reference attached to a recipe.
type OrganizerRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name"`
	Slug string `json:"slug,omitempty"`
}

// NamedRef is a minimal id/name reference (e.g. a food or unit on an ingredient).
type NamedRef struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// ListOptions are the pagination/ordering/search parameters common to all list
// endpoints. Zero-valued fields are omitted from the request.
type ListOptions struct {
	Page           int
	PerPage        int
	OrderBy        string
	OrderDirection string
	QueryFilter    string
	Search         string
}

// All returns ListOptions that request every result in a single page
// (Mealie interprets perPage=-1 as "no pagination").
func All() ListOptions { return ListOptions{PerPage: -1} }

func (o ListOptions) apply(q url.Values) {
	if o.Page != 0 {
		q.Set("page", strconv.Itoa(o.Page))
	}
	if o.PerPage != 0 {
		q.Set("perPage", strconv.Itoa(o.PerPage))
	}
	if o.OrderBy != "" {
		q.Set("orderBy", o.OrderBy)
	}
	if o.OrderDirection != "" {
		q.Set("orderDirection", o.OrderDirection)
	}
	if o.QueryFilter != "" {
		q.Set("queryFilter", o.QueryFilter)
	}
	if o.Search != "" {
		q.Set("search", o.Search)
	}
}
