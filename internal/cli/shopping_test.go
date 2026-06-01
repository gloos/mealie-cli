package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestShoppingRecipeAdd(t *testing.T) {
	var sawGet, sawPost bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/curry":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"r1","slug":"curry","name":"Curry"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/lists/l1/recipe":
			sawPost = true
			_, _ = w.Write([]byte(`{"id":"l1","name":"Weekly","listItems":[{"id":"i1","shoppingListId":"l1","note":"2 onions"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, err := runRoot(t, nil, "",
		"shopping", "recipe", "add", "curry", "--list", "l1",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("shopping recipe add failed: %v\nstderr:\n%s", err, stderr)
	}
	if !sawGet {
		t.Error("expected a GET /api/recipes/curry to resolve the slug")
	}
	if !sawPost {
		t.Error("expected a POST to .../lists/l1/recipe")
	}
	// stdout must be just the list JSON (data only).
	var list map[string]any
	if uerr := json.Unmarshal([]byte(stdout), &list); uerr != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", uerr, stdout)
	}
	if list["name"] != "Weekly" {
		t.Errorf("list name = %v, want Weekly", list["name"])
	}
	// The "now N items" Info line goes to stderr, not stdout.
	if !strings.Contains(stderr, "Added recipe to list l1 (now 1 items)") {
		t.Errorf("expected the confirmation on stderr, got:\n%s", stderr)
	}
	if strings.Contains(stdout, "Added recipe") {
		t.Errorf("the Info line must not leak onto stdout:\n%s", stdout)
	}
}

func TestShoppingRecipeAddRejectsNonPositiveScale(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r) // must never be reached: the flag is rejected first
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"shopping", "recipe", "add", "curry", "--list", "l1", "--scale", "0",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err == nil {
		t.Fatal("expected a usage error for --scale 0, got nil")
	}
	if code, _ := classify(err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d (usage)", code, ExitUsage)
	}
}
