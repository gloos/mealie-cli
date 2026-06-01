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

	"github.com/gloos/mealie-cli/pkg/core"
)

// TestValidEntryType pins the meal-plan entry-type guard against the core enum:
// every documented type is accepted and an unknown one is rejected.
func TestValidEntryType(t *testing.T) {
	for _, et := range core.EntryTypes {
		if !validEntryType(et) {
			t.Errorf("validEntryType(%q) = false, want true", et)
		}
	}
	for _, bad := range []string{"", "brunch", "Dinner", "supper"} {
		if validEntryType(bad) {
			t.Errorf("validEntryType(%q) = true, want false", bad)
		}
	}
}

func TestMealplanList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/households/mealplans" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"page": 1, "per_page": 2, "total": 2, "total_pages": 1,
			"items": []map[string]any{
				{"id": 1, "date": "2026-06-01", "entryType": "dinner", "title": "Curry night"},
				{"id": 2, "date": "2026-06-02", "entryType": "lunch", "recipe": map[string]any{"slug": "soup", "name": "Soup"}},
			},
		})
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	t.Run("json", func(t *testing.T) {
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"mealplan", "list", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		var doc struct {
			Items []core.MealPlan `json:"items"`
		}
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
		}
		if len(doc.Items) != 2 || doc.Items[0].Title != "Curry night" {
			t.Errorf("unexpected items: %#v", doc.Items)
		}
	})

	t.Run("table renders recipe name fallback", func(t *testing.T) {
		stdout, _, code := runCLI(t, cliRun{args: []string{
			"mealplan", "list", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "table",
		}})
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		// The second entry has no title, so the table falls back to the recipe name.
		if !strings.Contains(stdout, "Curry night") || !strings.Contains(stdout, "Soup") {
			t.Errorf("table missing entries:\n%s", stdout)
		}
	})
}

func TestMealplanToday(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/households/mealplans/today" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`[{"id":7,"date":"2026-06-01","entryType":"dinner","title":"Tonight"}]`))
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "today", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var entries []core.MealPlan
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if len(entries) != 1 || entries[0].Title != "Tonight" {
		t.Errorf("unexpected today entries: %#v", entries)
	}
}

func TestMealplanAddFreeText(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/households/mealplans" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"id":42,"date":"2026-06-10","entryType":"lunch","title":"Picnic"}`))
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "add", "--date", "2026-06-10", "--type", "lunch", "--title", "Picnic",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if gotBody["entryType"] != "lunch" || gotBody["title"] != "Picnic" || gotBody["date"] != "2026-06-10" {
		t.Errorf("unexpected POST body: %#v", gotBody)
	}
	var entry core.MealPlan
	if err := json.Unmarshal([]byte(stdout), &entry); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if entry.ID != 42 {
		t.Errorf("entry id = %d, want 42", entry.ID)
	}
	if !strings.Contains(stderr, "Added lunch entry") {
		t.Errorf("expected confirmation on stderr, got:\n%s", stderr)
	}
}

func TestMealplanAddInvalidType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the invalid type must be rejected before any request (%s)", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "add", "--type", "brunch", "--title", "x",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d (usage)\nstderr:\n%s", code, ExitUsage, stderr)
	}
	pl, _ := decodeErrorEnvelope(t, stderr)
	if pl.Code != "usage" {
		t.Errorf("code = %q, want usage", pl.Code)
	}
}

func TestMealplanAddViaRecipeSlug(t *testing.T) {
	var sawGet, sawPost bool
	var postBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/curry":
			sawGet = true
			_, _ = w.Write([]byte(`{"id":"rid-1","slug":"curry","name":"Curry"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/households/mealplans":
			sawPost = true
			_ = json.NewDecoder(r.Body).Decode(&postBody)
			_, _ = w.Write([]byte(`{"id":9,"date":"2026-06-10","entryType":"dinner","recipeId":"rid-1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "add", "--date", "2026-06-10", "--recipe", "curry",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if !sawGet || !sawPost {
		t.Fatalf("expected slug GET (%v) then POST (%v)", sawGet, sawPost)
	}
	if postBody["recipeId"] != "rid-1" {
		t.Errorf("POST body recipeId = %v, want rid-1", postBody["recipeId"])
	}
}

func TestMealplanDelete(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/households/mealplans/42" {
			sawDelete = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "delete", "42", "--yes",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if !sawDelete {
		t.Fatal("expected DELETE .../mealplans/42")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %#v", doc)
	}
}

// paginatedMealplanServer serves /api/households/mealplans as `total` entries
// split into pages, optionally failing on a given page (for mid-stream tests).
func paginatedMealplanServer(t *testing.T, total, failOnPage int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/households/mealplans" {
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
			items = append(items, map[string]any{"id": i, "date": "2026-06-01", "entryType": "dinner", "title": fmt.Sprintf("Entry %d", i)})
		}
		totalPages := (total + perPage - 1) / perPage
		_ = json.NewEncoder(w).Encode(map[string]any{
			"page": page, "per_page": perPage, "total": total, "total_pages": totalPages, "items": items,
		})
	}))
}

func TestMealplanListAllPaginates(t *testing.T) {
	srv := paginatedMealplanServer(t, 5, 0)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, _, code := runCLI(t, cliRun{args: []string{
		"mealplan", "list", "--all", "--per-page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "ndjson",
	}})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 entries across 3 pages, got %d:\n%s", len(lines), stdout)
	}
}

func TestMealplanListAllMidStreamFailure(t *testing.T) {
	srv := paginatedMealplanServer(t, 10, 2)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "list", "--all", "--per-page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "ndjson",
	}})
	if code != ExitNetwork {
		t.Fatalf("exit = %d, want %d (network)\nstderr:\n%s", code, ExitNetwork, stderr)
	}
	if stdout != "" {
		t.Fatalf("a mid-stream failure must not leak the first page; stdout was:\n%s", stdout)
	}
}

func TestMealplanListAllRejectsPageAndNegativePerPage(t *testing.T) {
	srv := paginatedMealplanServer(t, 5, 0)
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	_, _, code := runCLI(t, cliRun{args: []string{
		"mealplan", "list", "--all", "--page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Errorf("--all --page exit = %d, want %d", code, ExitUsage)
	}

	_, _, code = runCLI(t, cliRun{args: []string{
		"mealplan", "list", "--all", "--per-page=-1",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Errorf("--all --per-page=-1 exit = %d, want %d", code, ExitUsage)
	}
}

func TestMealplanDeleteRejectsNonNumericID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("non-numeric id must be rejected before any request (%s)", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"mealplan", "delete", "abc", "--yes",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d (usage)\nstderr:\n%s", code, ExitUsage, stderr)
	}
}
