package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// paginatedRecipeServer serves /api/recipes as `total` recipes split into pages
// of the requested perPage, honouring the page query parameter so a client-side
// --all walk fetches every page.
func paginatedRecipeServer(t *testing.T, total int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/recipes" {
			http.NotFound(w, r)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
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
		items := make([]map[string]string, 0, perPage)
		for i := start; i < end && i < total; i++ {
			items = append(items, map[string]string{"slug": fmt.Sprintf("r%02d", i), "name": fmt.Sprintf("Recipe %d", i)})
		}
		totalPages := (total + perPage - 1) / perPage
		_ = json.NewEncoder(w).Encode(map[string]any{
			"page": page, "per_page": perPage, "total": total, "total_pages": totalPages, "items": items,
		})
	}))
}

func TestRecipeListAllPaginatesClientSide(t *testing.T) {
	srv := paginatedRecipeServer(t, 5)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, _, err := runRoot(t, nil, "",
		"recipe", "list", "--all", "--per-page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
		"--output", "ndjson",
	)
	if err != nil {
		t.Fatalf("recipe list --all failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	if len(lines) != 5 {
		t.Fatalf("expected 5 ndjson items across 3 pages, got %d:\n%s", len(lines), stdout)
	}
	// Spot-check that the items are the distinct recipes, not a repeated page.
	for i, line := range lines {
		var rec map[string]any
		if uerr := json.Unmarshal([]byte(line), &rec); uerr != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, uerr)
		}
		if rec["slug"] != fmt.Sprintf("r%02d", i) {
			t.Fatalf("line %d slug = %v, want r%02d", i, rec["slug"], i)
		}
	}
}

func TestRecipeListAllRejectsExplicitPage(t *testing.T) {
	srv := paginatedRecipeServer(t, 5)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "list", "--all", "--page", "2",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
		"--output", "json",
	)
	if err == nil {
		t.Fatal("expected a usage error for --all --page, got nil")
	}
	if code, _ := classify(err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d (usage)", code, ExitUsage)
	}
}

// recipeExportServer serves a fixed set of recipes for export: GET
// /api/recipes/{slug} returns a body tagged with the slug, and GET /api/recipes
// lists them (paginated) for --all.
func recipeExportServer(t *testing.T, slugs ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/recipes":
			items := make([]map[string]string, 0, len(slugs))
			for _, s := range slugs {
				items = append(items, map[string]string{"slug": s, "name": s})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "per_page": len(items), "total": len(items), "total_pages": 1, "items": items,
			})
		case strings.HasPrefix(r.URL.Path, "/api/recipes/"):
			slug := strings.TrimPrefix(r.URL.Path, "/api/recipes/")
			fmt.Fprintf(w, `{"slug":%q,"name":%q,"extras":{"k":"v"}}`, slug, slug)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestRecipeExportSingleToStdout(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, _, err := runRoot(t, nil, "",
		"recipe", "export", "curry",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err != nil {
		t.Fatalf("recipe export failed: %v", err)
	}
	var rec map[string]any
	if uerr := json.Unmarshal([]byte(stdout), &rec); uerr != nil {
		t.Fatalf("stdout is not valid JSON: %v\n%s", uerr, stdout)
	}
	if rec["slug"] != "curry" || rec["extras"] == nil {
		t.Fatalf("unexpected exported recipe (lossless fields missing?): %v", rec)
	}
}

func TestRecipeExportSingleToFile(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "curry.json")
	cfg := filepath.Join(dir, "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "curry", "-O", dest,
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err != nil {
		t.Fatalf("recipe export -O file failed: %v", err)
	}
	data, rerr := os.ReadFile(dest)
	if rerr != nil {
		t.Fatalf("export file not written: %v", rerr)
	}
	var rec map[string]any
	if uerr := json.Unmarshal(data, &rec); uerr != nil {
		t.Fatalf("file is not valid JSON: %v", uerr)
	}
	if rec["slug"] != "curry" {
		t.Fatalf("file slug = %v, want curry", rec["slug"])
	}
}

func TestRecipeExportOverwriteGuard(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "curry.json")
	cfg := filepath.Join(dir, "config.yaml")
	if werr := os.WriteFile(dest, []byte(`{"old":true}`), 0o644); werr != nil {
		t.Fatal(werr)
	}

	// Without --force the export must refuse, leaving the existing file untouched.
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "curry", "-O", dest,
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err == nil {
		t.Fatal("expected an overwrite error without --force")
	}
	if code, _ := classify(err); code != ExitConflict {
		t.Fatalf("exit code = %d, want %d (conflict)", code, ExitConflict)
	}
	if data, _ := os.ReadFile(dest); !strings.Contains(string(data), `"old":true`) {
		t.Fatalf("existing file must be untouched without --force, got %s", data)
	}

	// With --force the export overwrites.
	_, _, err = runRoot(t, nil, "",
		"recipe", "export", "curry", "-O", dest, "--force",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err != nil {
		t.Fatalf("export --force failed: %v", err)
	}
	data, _ := os.ReadFile(dest)
	if strings.Contains(string(data), `"old":true`) {
		t.Fatalf("--force should have overwritten the file, got %s", data)
	}
}

func TestRecipeExportAllToDir(t *testing.T) {
	srv := recipeExportServer(t, "curry", "pasta", "soup")
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "backup")
	cfg := filepath.Join(dir, "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "--all", "-O", out+"/",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
		"--output", "json",
	)
	if err != nil {
		t.Fatalf("recipe export --all failed: %v", err)
	}
	for _, slug := range []string{"curry", "pasta", "soup"} {
		if _, serr := os.Stat(filepath.Join(out, slug+".json")); serr != nil {
			t.Errorf("expected %s.json to be written: %v", slug, serr)
		}
	}
}

// TestRecipeExportFilePermissions guards the fix for the world-readable backup
// finding: exported recipes are lossless and can carry private data, so the
// written files must not be group- or world-readable.
func TestRecipeExportFilePermissions(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.yaml")
	// Cover both the single-file and the --all/dir write paths.
	file := filepath.Join(dir, "curry.json")
	if _, _, err := runRoot(t, nil, "",
		"recipe", "export", "curry", "-O", file,
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	); err != nil {
		t.Fatalf("export to file failed: %v", err)
	}
	out := filepath.Join(dir, "backup")
	if _, _, err := runRoot(t, nil, "",
		"recipe", "export", "--all", "-O", out+"/",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	); err != nil {
		t.Fatalf("export --all failed: %v", err)
	}
	for _, path := range []string{file, filepath.Join(out, "curry.json")} {
		info, serr := os.Stat(path)
		if serr != nil {
			t.Fatalf("stat %s: %v", path, serr)
		}
		if perm := info.Mode().Perm(); perm&0o077 != 0 {
			t.Errorf("%s mode = %o, must not be group/world accessible", path, perm)
		}
	}
}

// TestRecipeExportAllAtomicOnFetchError guards the stage-then-publish fix: if a
// recipe fetch fails mid-run, no destination is published and any pre-existing
// backup is left untouched, even under --force.
func TestRecipeExportAllAtomicOnFetchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/recipes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "per_page": 2, "total": 2, "total_pages": 1,
				"items": []map[string]string{{"slug": "good", "name": "good"}, {"slug": "bad", "name": "bad"}},
			})
		case r.URL.Path == "/api/recipes/good":
			fmt.Fprint(w, `{"slug":"good","name":"good"}`)
		case r.URL.Path == "/api/recipes/bad":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "backup")
	if mkErr := os.MkdirAll(out, 0o755); mkErr != nil {
		t.Fatal(mkErr)
	}
	// A pre-existing backup that must survive a failed --force run untouched.
	stale := filepath.Join(out, "good.json")
	if werr := os.WriteFile(stale, []byte(`{"stale":true}`), 0o600); werr != nil {
		t.Fatal(werr)
	}

	cfg := filepath.Join(dir, "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "--all", "-O", out+"/", "--force",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err == nil {
		t.Fatal("expected an error when a recipe fetch fails mid-run")
	}
	// good.json must still hold the stale bytes — nothing was published.
	data, _ := os.ReadFile(stale)
	if !strings.Contains(string(data), `"stale":true`) {
		t.Fatalf("pre-existing backup was modified despite the failed run: %s", data)
	}
	// No torn temp files left behind.
	entries, _ := os.ReadDir(out)
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("leftover staging temp file: %s", e.Name())
		}
	}
}

func TestRecipeExportRejectsUnsafeSlug(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	dir := t.TempDir()
	out := filepath.Join(dir, "backup") + "/"
	cfg := filepath.Join(dir, "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "../evil", "-O", out,
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err == nil {
		t.Fatal("expected a usage error for an unsafe slug")
	}
	if code, _ := classify(err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d (usage)", code, ExitUsage)
	}
	// The traversal target must never be created.
	if _, serr := os.Stat(filepath.Join(dir, "evil.json")); serr == nil {
		t.Fatal("an unsafe slug must be rejected before any write")
	}
}

func TestRecipeExportRejectsNDJSON(t *testing.T) {
	srv := recipeExportServer(t, "curry")
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "export", "curry", "--output", "ndjson",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
	)
	if err == nil {
		t.Fatal("expected a usage error for --output ndjson")
	}
	if code, _ := classify(err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d (usage)", code, ExitUsage)
	}
}

// TestRecipeGet drives the curated single-recipe fetch: the GET path is hit and
// the recipe lands on stdout as data only.
func TestRecipeGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/recipes/curry" {
			_, _ = w.Write([]byte(`{"id":"r1","slug":"curry","name":"Curry","recipeYield":"4 servings"}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"recipe", "get", "curry", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var rec map[string]any
	if uerr := json.Unmarshal([]byte(stdout), &rec); uerr != nil {
		t.Fatalf("stdout not JSON: %v\n%s", uerr, stdout)
	}
	if rec["slug"] != "curry" || rec["recipeYield"] != "4 servings" {
		t.Errorf("unexpected recipe: %#v", rec)
	}
}

// TestRecipeCreate covers the create success path (POST verb+body, slug on
// stdout, Info on stderr) and the validation failure mode (422 → exit 6 with the
// per-field details surfaced).
func TestRecipeCreate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		var gotBody map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost && r.URL.Path == "/api/recipes" {
				_ = json.NewDecoder(r.Body).Decode(&gotBody)
				_, _ = w.Write([]byte(`"curry"`))
				return
			}
			http.NotFound(w, r)
		}))
		defer srv.Close()

		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"recipe", "create", "Curry", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != 0 {
			t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
		}
		if gotBody["name"] != "Curry" {
			t.Errorf("POST body name = %v, want Curry", gotBody["name"])
		}
		var doc map[string]string
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
		}
		if doc["slug"] != "curry" {
			t.Errorf("slug = %q, want curry", doc["slug"])
		}
		if !strings.Contains(stderr, "Created recipe") {
			t.Errorf("expected Info on stderr, got:\n%s", stderr)
		}
	})

	t.Run("validation failure", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"detail":[{"loc":["body","name"],"msg":"field required"}]}`))
		}))
		defer srv.Close()

		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"recipe", "create", "x", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != ExitValidation {
			t.Fatalf("exit = %d, want %d (validation)\nstderr:\n%s", code, ExitValidation, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout must be empty on error, got:\n%s", stdout)
		}
		pl, raw := decodeErrorEnvelope(t, stderr)
		if pl.Code != "validation" {
			t.Errorf("code = %q, want validation", pl.Code)
		}
		if raw["details"] == nil {
			t.Errorf("expected details.fields, got none")
		}
	})
}

func TestRecipeImport(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/recipes/create/url" {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`"scraped-dish"`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"recipe", "import", "https://example.com/recipe",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	if gotBody["url"] != "https://example.com/recipe" {
		t.Errorf("POST body url = %v", gotBody["url"])
	}
	var doc map[string]string
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["slug"] != "scraped-dish" {
		t.Errorf("slug = %q, want scraped-dish", doc["slug"])
	}
}

func TestRecipeListAllRejectsNegativePerPage(t *testing.T) {
	srv := paginatedRecipeServer(t, 5)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, _, err := runRoot(t, nil, "",
		"recipe", "list", "--all", "--per-page=-1",
		"--url", srv.URL, "--token", "tok", "--config", cfg,
		"--output", "json",
	)
	if err == nil {
		t.Fatal("expected a usage error for --all --per-page=-1, got nil")
	}
	if code, _ := classify(err); code != ExitUsage {
		t.Fatalf("exit code = %d, want %d (usage)", code, ExitUsage)
	}
}
