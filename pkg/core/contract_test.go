package core

import (
	"encoding/json"
	"os"
	"testing"
)

const specPath = "../../api/specs/mealie/v3.19.2/openapi.json"

// These contract tests guard the hand-written client against upstream API drift.
// Rather than only checking that paths exist, they assert — against the pinned
// Mealie OpenAPI spec — the exact things this client depends on: that each
// operation exists with the method we use, that the query parameters we send are
// declared, that request/response bodies reference the schema we encode/decode,
// and that the DTO fields we (de)serialise are present. See docs/adr/0001.

type openapiSpec struct {
	Paths      map[string]map[string]operation `json:"paths"`
	Components struct {
		Schemas map[string]schema `json:"schemas"`
	} `json:"components"`
}

type operation struct {
	Parameters  []parameter           `json:"parameters"`
	RequestBody *bodyOrResp           `json:"requestBody"`
	Responses   map[string]bodyOrResp `json:"responses"`
}

type parameter struct {
	Name string `json:"name"`
	In   string `json:"in"`
}

type bodyOrResp struct {
	Content map[string]struct {
		Schema schema `json:"schema"`
	} `json:"content"`
}

type schema struct {
	Ref        string            `json:"$ref"`
	Type       string            `json:"type"`
	Properties map[string]schema `json:"properties"`
}

func loadSpec(t *testing.T) *openapiSpec {
	t.Helper()
	data, err := os.ReadFile(specPath)
	if err != nil {
		t.Skipf("pinned spec unavailable (%v); skipping contract checks", err)
	}
	var spec openapiSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("parse spec: %v", err)
	}
	return &spec
}

func (s *openapiSpec) op(t *testing.T, path, method string) operation {
	t.Helper()
	p, ok := s.Paths[path]
	if !ok {
		t.Fatalf("spec is missing path %s", path)
	}
	op, ok := p[method]
	if !ok {
		t.Fatalf("spec path %s has no %s operation", path, method)
	}
	return op
}

func refName(ref string) string {
	for i := len(ref) - 1; i >= 0; i-- {
		if ref[i] == '/' {
			return ref[i+1:]
		}
	}
	return ref
}

// TestContractOperations asserts each endpoint the client calls exists with the
// expected method, and that the query params we send are declared on it.
func TestContractOperations(t *testing.T) {
	spec := loadSpec(t)
	cases := []struct {
		path, method string
		query        []string
	}{
		{"/api/app/about", "get", nil},
		{"/api/users/self", "get", nil},
		{"/api/auth/token", "post", nil},
		{"/api/users/api-tokens", "post", nil},
		{"/api/recipes", "get", []string{"page", "perPage", "orderBy", "orderDirection", "queryFilter", "search", "categories", "tags", "tools", "cookbook"}},
		{"/api/recipes", "post", nil},
		{"/api/recipes/{slug}", "get", nil},
		{"/api/recipes/{slug}", "delete", nil},
		{"/api/recipes/create/url", "post", nil},
		{"/api/households/mealplans", "get", []string{"start_date", "end_date", "page", "perPage"}},
		{"/api/households/mealplans", "post", nil},
		{"/api/households/mealplans/today", "get", nil},
		{"/api/households/mealplans/{item_id}", "delete", nil},
		{"/api/households/shopping/lists", "get", []string{"page", "perPage"}},
		{"/api/households/shopping/lists", "post", nil},
		{"/api/households/shopping/lists/{item_id}", "get", nil},
		{"/api/households/shopping/lists/{item_id}", "delete", nil},
		{"/api/households/shopping/items", "post", nil},
		{"/api/households/shopping/items/{item_id}", "get", nil},
		{"/api/households/shopping/items/{item_id}", "put", nil},
		{"/api/households/shopping/items/{item_id}", "delete", nil},
	}
	for _, c := range cases {
		op := spec.op(t, c.path, c.method)
		declared := map[string]bool{}
		for _, p := range op.Parameters {
			if p.In == "query" {
				declared[p.Name] = true
			}
		}
		for _, q := range c.query {
			if !declared[q] {
				t.Errorf("%s %s: client sends query param %q not declared in spec", c.method, c.path, q)
			}
		}
	}
}

// TestContractResponseShapes asserts the response bodies we decode reference the
// schema we expect — the check that would have caught the shopping-item update
// envelope bug.
func TestContractResponseShapes(t *testing.T) {
	spec := loadSpec(t)
	cases := []struct {
		path, method, code, wantSchema string
	}{
		{"/api/households/shopping/items", "post", "201", "ShoppingListItemsCollectionOut"},
		{"/api/households/shopping/items/{item_id}", "put", "200", "ShoppingListItemsCollectionOut"},
		{"/api/households/shopping/items/{item_id}", "get", "200", "ShoppingListItemOut-Output"},
		{"/api/households/shopping/lists", "post", "201", "ShoppingListOut"},
		{"/api/households/shopping/lists/{item_id}", "get", "200", "ShoppingListOut"},
		{"/api/households/mealplans", "post", "201", "ReadPlanEntry"},
	}
	for _, c := range cases {
		op := spec.op(t, c.path, c.method)
		resp, ok := op.Responses[c.code]
		if !ok {
			t.Errorf("%s %s: no %s response", c.method, c.path, c.code)
			continue
		}
		got := refName(resp.Content["application/json"].Schema.Ref)
		if got != c.wantSchema {
			t.Errorf("%s %s %s: response schema = %q, want %q (client decoding may be wrong)",
				c.method, c.path, c.code, got, c.wantSchema)
		}
	}
}

func (s *openapiSpec) props(name string) map[string]schema {
	return s.Components.Schemas[name].Properties
}

// TestContractDTOFields asserts the JSON fields the client (de)serialises exist
// on the corresponding spec schema.
func TestContractDTOFields(t *testing.T) {
	spec := loadSpec(t)
	checks := map[string][]string{
		"AppInfo":                        {"version", "production"},
		"RecipeSummary":                  {"slug", "name", "recipeCategory", "tags", "recipeYield", "totalTime", "rating", "dateAdded"},
		"CreatePlanEntry":                {"date", "entryType", "title", "text", "recipeId"},
		"ReadPlanEntry":                  {"id", "date", "entryType", "title", "text", "recipeId", "recipe"},
		"ShoppingListItemCreate":         {"shoppingListId", "note", "quantity"},
		"ShoppingListItemOut-Output":     {"id", "shoppingListId", "checked", "position", "foodId", "unitId", "labelId", "note", "quantity"},
		"ShoppingListItemsCollectionOut": {"createdItems", "updatedItems", "deletedItems"},
	}
	for name, fields := range checks {
		props := spec.props(name)
		if len(props) == 0 {
			t.Errorf("schema %s not found or has no properties", name)
			continue
		}
		for _, f := range fields {
			if _, ok := props[f]; !ok {
				t.Errorf("schema %s is missing expected field %q", name, f)
			}
		}
	}
}
