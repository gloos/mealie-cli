package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
