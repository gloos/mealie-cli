package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
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

func TestShoppingList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/households/shopping/lists" {
			_, _ = w.Write([]byte(`{"page":1,"per_page":1,"total":1,"total_pages":1,"items":[{"id":"l1","name":"Weekly"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "list", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"Weekly"`) {
		t.Errorf("expected the list on stdout, got:\n%s", stdout)
	}
}

func TestShoppingGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/households/shopping/lists/l1" {
			_, _ = w.Write([]byte(`{"id":"l1","name":"Weekly","listItems":[{"id":"i1","note":"Eggs"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "get", "l1", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var list map[string]any
	if err := json.Unmarshal([]byte(stdout), &list); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if list["name"] != "Weekly" {
		t.Errorf("name = %v, want Weekly", list["name"])
	}
}

func TestShoppingGetNotFound(t *testing.T) {
	srv := statusServer(404, `{"detail":"no such list"}`)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "get", "missing", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitNotFound {
		t.Fatalf("exit = %d, want %d\nstderr:\n%s", code, ExitNotFound, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty, got:\n%s", stdout)
	}
}

func TestShoppingCreate(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/lists" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"id":"l9","name":"Party"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "create", "Party", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if gotBody["name"] != "Party" {
		t.Errorf("POST body name = %v, want Party", gotBody["name"])
	}
	if !strings.Contains(stdout, `"l9"`) {
		t.Errorf("expected the new list id on stdout, got:\n%s", stdout)
	}
}

func TestShoppingDelete(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/households/shopping/lists/l1" {
			sawDelete = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "delete", "l1", "--yes", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if !sawDelete {
		t.Fatal("expected DELETE .../lists/l1")
	}
}

func TestShoppingItemAdd(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/items" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"createdItems":[{"id":"i1","shoppingListId":"l1","note":"2 onions"}]}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "item", "add", "2", "onions", "--list", "l1",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if gotBody["shoppingListId"] != "l1" || gotBody["note"] != "2 onions" {
		t.Errorf("unexpected POST body: %#v", gotBody)
	}
	if !strings.Contains(stdout, `"i1"`) {
		t.Errorf("expected the created item on stdout, got:\n%s", stdout)
	}
}

func TestShoppingItemCheck(t *testing.T) {
	var sawGet, sawPut bool
	var putBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/households/shopping/items/i1" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"i1","shoppingListId":"l1","checked":false,"note":"Eggs","labelId":"lbl-1"}`))
		case http.MethodPut:
			sawPut = true
			_ = json.NewDecoder(r.Body).Decode(&putBody)
			_, _ = w.Write([]byte(`{"updatedItems":[{"id":"i1","shoppingListId":"l1","checked":true,"note":"Eggs"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "item", "check", "i1", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if !sawGet || !sawPut {
		t.Fatalf("check should GET then PUT (get=%v put=%v)", sawGet, sawPut)
	}
	// The round-trip must preserve fields outside the curated struct (labelId) and
	// flip checked to true.
	if putBody["labelId"] != "lbl-1" || putBody["checked"] != true {
		t.Errorf("PUT body did not preserve+update fields: %#v", putBody)
	}
	var item map[string]any
	if err := json.Unmarshal([]byte(stdout), &item); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if item["checked"] != true {
		t.Errorf("expected checked=true, got %#v", item)
	}
}

func TestShoppingItemDelete(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/households/shopping/items/i1" {
			sawDelete = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "item", "delete", "i1", "--yes", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if !sawDelete {
		t.Fatal("expected DELETE .../items/i1")
	}
}

// TestShoppingRecipeAddSlugNotFound exercises the recipe-add failure mode where
// the slug lookup 404s: the command must exit 4 and never POST to the list.
func TestShoppingRecipeAddSlugNotFound(t *testing.T) {
	var sawPost bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/missing":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		case r.Method == http.MethodPost:
			sawPost = true
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "recipe", "add", "missing", "--list", "l1",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitNotFound {
		t.Fatalf("exit = %d, want %d (not_found)\nstderr:\n%s", code, ExitNotFound, stderr)
	}
	if sawPost {
		t.Fatal("must not POST to the list when the slug lookup fails")
	}
}

// TestShoppingRecipeAddPostValidation exercises the failure mode where the slug
// resolves but the list POST is rejected with 422 → exit 6.
func TestShoppingRecipeAddPostValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/curry":
			_, _ = w.Write([]byte(`{"id":"r1","slug":"curry","name":"Curry"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/lists/l1/recipe":
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":[{"loc":["body","recipeId"],"msg":"invalid"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "recipe", "add", "curry", "--list", "l1",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitValidation {
		t.Fatalf("exit = %d, want %d (validation)\nstderr:\n%s", code, ExitValidation, stderr)
	}
	pl, _ := decodeErrorEnvelope(t, stderr)
	if pl.Code != "validation" {
		t.Errorf("code = %q, want validation", pl.Code)
	}
}

// TestShoppingRecipeAddByID confirms --recipe-id skips the slug GET and posts the
// id straight to the list.
func TestShoppingRecipeAddByID(t *testing.T) {
	var sawGet bool
	var postBody []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/recipes/"):
			sawGet = true
			http.NotFound(w, r)
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/lists/l1/recipe":
			_ = json.NewDecoder(r.Body).Decode(&postBody)
			_, _ = w.Write([]byte(`{"id":"l1","name":"Weekly","listItems":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "recipe", "add", "curry", "--list", "l1", "--recipe-id", "rid-7",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if sawGet {
		t.Error("--recipe-id must skip the slug GET lookup")
	}
	if len(postBody) != 1 || postBody[0]["recipeId"] != "rid-7" {
		t.Errorf("unexpected POST body: %#v", postBody)
	}
}

// paginatedShoppingServer serves /api/households/shopping/lists as `total` lists
// split into requested pages. When failOnPage > 0, that page responds 500 — used
// to prove a mid-stream failure aborts the whole --all walk with nothing leaked.
func paginatedShoppingServer(t *testing.T, total, failOnPage int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/households/shopping/lists" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		if failOnPage > 0 && page == failOnPage {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
		if perPage < 1 {
			perPage = 50
		}
		start := (page - 1) * perPage
		end := start + perPage
		if end > total {
			end = total
		}
		items := make([]map[string]any, 0, perPage)
		for i := start; i < end && i < total; i++ {
			items = append(items, map[string]any{"id": fmt.Sprintf("l%02d", i), "name": fmt.Sprintf("List %d", i)})
		}
		totalPages := (total + perPage - 1) / perPage
		_ = json.NewEncoder(w).Encode(map[string]any{
			"page": page, "per_page": perPage, "total": total, "total_pages": totalPages, "items": items,
		})
	}))
}

func TestShoppingListAllPaginates(t *testing.T) {
	srv := paginatedShoppingServer(t, 5, 0)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, _, code := runCLI(t, cliRun{args: []string{
		"shopping", "list", "--all", "--per-page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "ndjson",
	}})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 lists across 3 pages, got %d:\n%s", len(lines), stdout)
	}
}

// TestShoppingListAllMidStreamFailure proves a page-2 failure aborts the walk:
// the command exits 7 and leaks nothing onto stdout (no partial first page).
func TestShoppingListAllMidStreamFailure(t *testing.T) {
	srv := paginatedShoppingServer(t, 10, 2)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"shopping", "list", "--all", "--per-page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "ndjson",
	}})
	if code != ExitNetwork {
		t.Fatalf("exit = %d, want %d (network)\nstderr:\n%s", code, ExitNetwork, stderr)
	}
	if stdout != "" {
		t.Fatalf("a mid-stream failure must not leak the first page; stdout was:\n%s", stdout)
	}
}

func TestShoppingListAllRejectsPageAndNegativePerPage(t *testing.T) {
	srv := paginatedShoppingServer(t, 5, 0)
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	// --all with explicit --page is a usage error.
	_, _, code := runCLI(t, cliRun{args: []string{
		"shopping", "list", "--all", "--page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Errorf("--all --page exit = %d, want %d", code, ExitUsage)
	}

	// --all with a negative --per-page is a usage error.
	_, _, code = runCLI(t, cliRun{args: []string{
		"shopping", "list", "--all", "--per-page=-1",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Errorf("--all --per-page=-1 exit = %d, want %d", code, ExitUsage)
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
