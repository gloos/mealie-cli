package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestAboutSendsAuthAndUserAgent(t *testing.T) {
	var gotAuth, gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		if r.URL.Path != "/api/app/about" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"version":"v3.19.2","production":true}`))
	}))
	defer srv.Close()

	c, err := New(srv.URL, "tok", WithUserAgent("mealie-cli/test"))
	if err != nil {
		t.Fatal(err)
	}
	about, err := c.About(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if about.Version != "v3.19.2" || !about.Production {
		t.Fatalf("unexpected about: %+v", about)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if gotUA != "mealie-cli/test" {
		t.Errorf("User-Agent = %q", gotUA)
	}
	if gotAccept != "application/json" {
		t.Errorf("Accept = %q", gotAccept)
	}
}

func TestNotFoundMapsToAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"detail":"recipe not found"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	_, err := c.GetRecipe(context.Background(), "missing")
	if !IsNotFound(err) {
		t.Fatalf("expected not-found, got %v", err)
	}
	apiErr, ok := AsAPIError(err)
	if !ok || apiErr.Code != "not_found" || apiErr.Message != "recipe not found" {
		t.Fatalf("unexpected api error: %+v", apiErr)
	}
}

func TestValidationErrorExtractsFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":[{"loc":["body","name"],"msg":"field required","type":"missing"}]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	_, err := c.CreateRecipe(context.Background(), "")
	if !IsValidation(err) {
		t.Fatalf("expected validation error, got %v", err)
	}
	apiErr, _ := AsAPIError(err)
	if len(apiErr.Fields) != 1 || apiErr.Fields[0].Location != "name" {
		t.Fatalf("unexpected fields: %+v", apiErr.Fields)
	}
}

func TestRetryOnTransientGET(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"version":"v3.19.2"}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok", WithMaxRetries(3))
	c.backoffBase = 0 // no real sleeps in tests
	if _, err := c.About(context.Background()); err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("expected 3 calls, got %d", n)
	}
}

func TestNoRetryOnPOST(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok", WithMaxRetries(3))
	if _, err := c.CreateRecipe(context.Background(), "x"); err == nil {
		t.Fatal("expected error")
	}
	if n := atomic.LoadInt32(&calls); n != 1 {
		t.Fatalf("POST should not retry; got %d calls", n)
	}
}

func TestSetItemCheckedPreservesFields(t *testing.T) {
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_, _ = w.Write([]byte(`{"id":"i1","shoppingListId":"l1","checked":false,"position":3,` +
				`"foodId":"f1","unitId":"u1","labelId":"lbl1","quantity":2,"extras":{"k":"v"},"note":"milk"}`))
		case http.MethodPut:
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_, _ = w.Write([]byte(`{"updatedItems":[{"id":"i1","shoppingListId":"l1","checked":true,"note":"milk"}]}`))
		default:
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	updated, err := c.SetItemChecked(context.Background(), "i1", true)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.Checked {
		t.Fatal("expected updated item to be checked")
	}
	if putBody["checked"] != true {
		t.Fatalf("PUT body did not set checked=true: %v", putBody["checked"])
	}
	// The whole point of the round-trip: fields outside the curated struct must
	// survive the update rather than being nulled out.
	for _, k := range []string{"foodId", "unitId", "labelId", "position", "extras", "quantity", "shoppingListId"} {
		if _, ok := putBody[k]; !ok {
			t.Errorf("PUT body dropped preserved field %q", k)
		}
	}
}

func TestAddShoppingItemDecodesCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "unexpected", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"createdItems":[{"id":"new1","shoppingListId":"l1","note":"2 onions"}],"updatedItems":[],"deletedItems":[]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	item, err := c.AddShoppingItem(context.Background(), CreateShoppingItem{ShoppingListID: "l1", Note: "2 onions"})
	if err != nil {
		t.Fatal(err)
	}
	if item.ID != "new1" || item.Note != "2 onions" {
		t.Fatalf("unexpected created item: %+v", item)
	}
}

func TestExportRecipeReturnsRawBody(t *testing.T) {
	// A field the curated Recipe struct does not model, to prove export is lossless.
	const body = `{"id":"r1","slug":"curry","name":"Curry","settings":{"public":true},"extras":{"k":"v"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/recipes/curry" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	raw, err := c.ExportRecipe(context.Background(), "curry")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != body {
		t.Fatalf("raw body = %s\nwant %s", raw, body)
	}
}

func TestAddRecipesToShoppingList(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id":"l1","name":"Weekly","listItems":[` +
			`{"id":"i1","shoppingListId":"l1","note":"2 onions"},` +
			`{"id":"i2","shoppingListId":"l1","note":"1 tin tomatoes"}]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	list, err := c.AddRecipesToShoppingList(context.Background(), "l1", []AddRecipeToList{{RecipeID: "r1"}})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %q, want POST", gotMethod)
	}
	if gotPath != "/api/households/shopping/lists/l1/recipe" {
		t.Errorf("path = %q", gotPath)
	}
	// The body must be an array (the non-deprecated bulk endpoint), with a single
	// element here.
	if len(gotBody) != 1 {
		t.Fatalf("request body should be a one-element array, got %v", gotBody)
	}
	if gotBody[0]["recipeId"] != "r1" {
		t.Errorf("recipeId = %v, want r1", gotBody[0]["recipeId"])
	}
	// An unset scale must be omitted so the server applies its default.
	if _, ok := gotBody[0]["recipeIncrementQuantity"]; ok {
		t.Errorf("recipeIncrementQuantity must be omitted when scale is unset; body = %v", gotBody[0])
	}
	if list.Name != "Weekly" || len(list.ListItems) != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}
}

// TestAddRecipeToListScaleEncoding pins the omitempty trade-off the request
// struct depends on: an unset scale is dropped (server default applies) while an
// explicit scale is sent.
func TestAddRecipeToListScaleEncoding(t *testing.T) {
	b, err := json.Marshal(AddRecipeToList{RecipeID: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "recipeIncrementQuantity") {
		t.Errorf("unset scale must be omitted, got %s", b)
	}
	b, err = json.Marshal(AddRecipeToList{RecipeID: "r1", Scale: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"recipeIncrementQuantity":2`) {
		t.Errorf("explicit scale must be sent, got %s", b)
	}
}

func TestAddRecipesToShoppingListValidationError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"detail":[{"loc":["body",0,"recipeId"],"msg":"field required","type":"missing"}]}`))
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "tok")
	_, err := c.AddRecipesToShoppingList(context.Background(), "l1", []AddRecipeToList{{}})
	if !IsValidation(err) {
		t.Fatalf("expected a validation error, got %v", err)
	}
}

func TestLoginMintsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/token":
			_ = r.ParseForm()
			if r.Form.Get("username") != "me" || r.Form.Get("password") != "pw" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"acc","token_type":"bearer"}`))
		case "/api/users/api-tokens":
			if r.Header.Get("Authorization") != "Bearer acc" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"token":"long-lived","name":"cli","id":"1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c, _ := New(srv.URL, "")
	tok, err := c.Login(context.Background(), "me", "pw", "cli")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "long-lived" {
		t.Fatalf("token = %q", tok)
	}
}
