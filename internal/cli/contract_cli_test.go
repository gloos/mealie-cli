package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

// ---------------------------------------------------------------------------
// A.1 — classify table: the exit-code mapping, the heart of the agent contract.
// ---------------------------------------------------------------------------

// TestClassifyTable pins the full classify contract: every error class an
// upstream response or an internal failure can take maps onto a specific exit
// code AND a specific stable payload. The exit class and the payload `code` are
// deliberately separate columns so they can never be conflated — e.g. a 422 and
// a 400 share exit 6 but carry different codes. The code/retryable columns mirror
// pkg/core (codeForStatus + parseAPIError's retryable rule); the real derivation
// from a live response is pinned end-to-end by TestErrorEnvelopeEndToEnd.
func TestClassifyTable(t *testing.T) {
	apiErr := func(status int, code string, retryable bool, fields ...core.FieldError) error {
		return &core.APIError{
			StatusCode: status,
			Code:       code,
			Message:    http.StatusText(status),
			RequestID:  "req-1",
			Retryable:  retryable,
			Fields:     fields,
		}
	}

	cases := []struct {
		name       string
		err        error
		wantExit   int
		wantCode   string
		wantRetry  bool
		wantHint   bool
		wantFields bool
		wantStatus int
	}{
		{"400 bad_request", apiErr(400, "bad_request", false), ExitValidation, "bad_request", false, false, false, 400},
		{"401 auth", apiErr(401, "auth", false), ExitConfig, "auth", false, true, false, 401},
		{"403 forbidden", apiErr(403, "forbidden", false), ExitConfig, "forbidden", false, true, false, 403},
		{"404 not_found", apiErr(404, "not_found", false), ExitNotFound, "not_found", false, false, false, 404},
		{"409 conflict", apiErr(409, "conflict", false), ExitConflict, "conflict", false, false, false, 409},
		{"422 validation", apiErr(422, "validation", false, core.FieldError{Location: "name", Message: "field required"}), ExitValidation, "validation", false, false, true, 422},
		{"429 rate_limited retryable", apiErr(429, "rate_limited", true), ExitNetwork, "rate_limited", true, false, false, 429},
		{"500 server_error non-retryable", apiErr(500, "server_error", false), ExitNetwork, "server_error", false, false, false, 500},
		{"503 server_error retryable", apiErr(503, "server_error", true), ExitNetwork, "server_error", true, false, false, 503},
		// Non-API rows.
		{"cliError passthrough", newError(ExitConflict, "exists", "already there", "use --force"), ExitConflict, "exists", false, true, false, 0},
		{"transport error", &core.TransportError{Err: errors.New("dial tcp: refused")}, ExitNetwork, "network", true, true, false, 0},
		{"bare error", errors.New("something broke"), ExitError, "error", false, false, false, 0},
		{"usage error", usageError("bad flag --frob"), ExitUsage, "usage", false, false, false, 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, pl := classify(c.err)
			if code != c.wantExit {
				t.Errorf("exit code = %d, want %d", code, c.wantExit)
			}
			if pl.Code != c.wantCode {
				t.Errorf("payload code = %q, want %q", pl.Code, c.wantCode)
			}
			if pl.Retryable != c.wantRetry {
				t.Errorf("retryable = %v, want %v", pl.Retryable, c.wantRetry)
			}
			if c.wantHint && pl.Hint == "" {
				t.Errorf("expected a hint, got none")
			}
			if !c.wantHint && pl.Hint != "" {
				t.Errorf("expected no hint, got %q", pl.Hint)
			}
			if c.wantStatus != 0 && pl.HTTPStatus != c.wantStatus {
				t.Errorf("http_status = %d, want %d", pl.HTTPStatus, c.wantStatus)
			}
			if c.wantFields {
				fields, ok := pl.Details["fields"].([]map[string]string)
				if !ok || len(fields) == 0 {
					t.Fatalf("expected details.fields, got %#v", pl.Details)
				}
				if fields[0]["location"] != "name" || fields[0]["message"] != "field required" {
					t.Errorf("field error not preserved: %#v", fields[0])
				}
			} else if pl.Details != nil {
				t.Errorf("expected no details, got %#v", pl.Details)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A.2 — full-triple end-to-end: exit code + whole-stderr envelope + clean stdout.
// ---------------------------------------------------------------------------

// statusServer responds to every request with the given status, body and an
// X-Request-Id header, so a command's first request fails the way the error
// contract expects. The fixed request id lets the propagation assertion bite.
func statusServer(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Request-Id", "req-xyz")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
}

// decodeErrorEnvelope unmarshals the *entire* stderr as a single JSON error
// envelope and fails on any leading or trailing bytes — a loose "contains an
// error object" check would pass even if a warning or human line were prepended,
// which would break the documented `2>err.json | jq` workflow. It returns both
// the typed payload and the raw map (for field-presence checks such as
// `retryable` being present even when false).
func decodeErrorEnvelope(t *testing.T, stderr string) (output.ErrorPayload, map[string]any) {
	t.Helper()
	trimmed := strings.TrimSpace(stderr)
	dec := json.NewDecoder(strings.NewReader(trimmed))
	var env struct {
		Error output.ErrorPayload `json:"error"`
	}
	if err := dec.Decode(&env); err != nil {
		t.Fatalf("stderr is not a single JSON error envelope: %v\nstderr:\n%s", err, stderr)
	}
	if dec.More() {
		t.Fatalf("stderr has trailing bytes after the error envelope:\n%s", stderr)
	}
	var raw map[string]map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		t.Fatalf("re-parse error envelope: %v", err)
	}
	return env.Error, raw["error"]
}

func TestErrorEnvelopeEndToEnd(t *testing.T) {
	const validationBody = `{"detail":[{"loc":["body","name"],"msg":"field required"}]}`
	const detailBody = `{"detail":"upstream said no"}`

	cases := []struct {
		name       string
		status     int
		body       string
		args       []string
		wantExit   int
		wantCode   string
		wantRetry  bool
		wantHint   bool
		wantFields bool
	}{
		{"400", 400, detailBody, []string{"recipe", "get", "x"}, ExitValidation, "bad_request", false, false, false},
		{"401", 401, detailBody, []string{"recipe", "get", "x"}, ExitConfig, "auth", false, true, false},
		{"403", 403, detailBody, []string{"recipe", "get", "x"}, ExitConfig, "forbidden", false, true, false},
		{"404", 404, detailBody, []string{"recipe", "get", "x"}, ExitNotFound, "not_found", false, false, false},
		{"409", 409, detailBody, []string{"recipe", "get", "x"}, ExitConflict, "conflict", false, false, false},
		// POST commands for 422/429/503: POST is never auto-retried, so the test
		// is one deterministic request with no backoff sleeps.
		{"422", 422, validationBody, []string{"recipe", "create", "Soup"}, ExitValidation, "validation", false, false, true},
		{"429", 429, detailBody, []string{"recipe", "create", "Soup"}, ExitNetwork, "rate_limited", true, false, false},
		{"500", 500, detailBody, []string{"recipe", "get", "x"}, ExitNetwork, "server_error", false, false, false},
		{"503", 503, detailBody, []string{"recipe", "create", "Soup"}, ExitNetwork, "server_error", true, false, false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			srv := statusServer(c.status, c.body)
			defer srv.Close()

			cfg := filepath.Join(t.TempDir(), "config.yaml")
			args := append([]string{}, c.args...)
			args = append(args, "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json")
			stdout, stderr, code := runCLI(t, cliRun{args: args})

			if code != c.wantExit {
				t.Fatalf("exit code = %d, want %d\nstderr:\n%s", code, c.wantExit, stderr)
			}
			// No partial data may leak onto stdout alongside an error.
			if stdout != "" {
				t.Fatalf("stdout must be empty on error, got:\n%s", stdout)
			}
			pl, raw := decodeErrorEnvelope(t, stderr)
			if pl.Code != c.wantCode {
				t.Errorf("code = %q, want %q", pl.Code, c.wantCode)
			}
			if pl.Message == "" {
				t.Errorf("message must not be empty")
			}
			if pl.HTTPStatus != c.status {
				t.Errorf("http_status = %d, want %d", pl.HTTPStatus, c.status)
			}
			if pl.Retryable != c.wantRetry {
				t.Errorf("retryable = %v, want %v", pl.Retryable, c.wantRetry)
			}
			// retryable must be present in the wire form even when false, so an agent
			// can read it unconditionally.
			if _, ok := raw["retryable"]; !ok {
				t.Errorf("retryable field absent from the envelope; it must always be present")
			}
			if pl.RequestID != "req-xyz" {
				t.Errorf("request_id = %q, want req-xyz (propagated from X-Request-Id)", pl.RequestID)
			}
			if c.wantHint && pl.Hint == "" {
				t.Errorf("expected a hint for %s", c.name)
			}
			if c.wantFields {
				fields, _ := raw["details"].(map[string]any)
				if fields == nil || fields["fields"] == nil {
					t.Errorf("expected details.fields for validation, got %#v", raw["details"])
				} else {
					list := fields["fields"].([]any)
					first := list[0].(map[string]any)
					if first["location"] != "name" || first["message"] != "field required" {
						t.Errorf("field error not surfaced: %#v", first)
					}
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// A.3 — success in every format: stdout parses, nothing leaks, stderr is clean.
// ---------------------------------------------------------------------------

func listServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/recipes" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"page": 1, "per_page": 2, "total": 2, "total_pages": 1,
			"items": []map[string]any{
				{"slug": "curry", "name": "Curry"},
				{"slug": "pasta", "name": "Pasta"},
			},
		})
	}))
}

func TestSuccessEveryFormat(t *testing.T) {
	srv := listServer()
	defer srv.Close()

	run := func(t *testing.T, format string) (string, string, int) {
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		return runCLI(t, cliRun{args: []string{
			"recipe", "list", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", format,
		}})
	}

	t.Run("json", func(t *testing.T) {
		stdout, stderr, code := run(t, "json")
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout is not a single JSON document: %v\n%s", err, stdout)
		}
		if doc["items"] == nil || doc["pagination"] == nil {
			t.Errorf("expected items+pagination, got %#v", doc)
		}
	})

	t.Run("ndjson", func(t *testing.T) {
		stdout, stderr, code := run(t, "ndjson")
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 ndjson lines, got %d:\n%s", len(lines), stdout)
		}
		for i, line := range lines {
			var rec map[string]any
			if err := json.Unmarshal([]byte(line), &rec); err != nil {
				t.Fatalf("ndjson line %d invalid: %v", i, err)
			}
		}
	})

	t.Run("yaml", func(t *testing.T) {
		stdout, stderr, code := run(t, "yaml")
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		var doc map[string]any
		if err := yaml.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout is not valid YAML: %v\n%s", err, stdout)
		}
		if doc["items"] == nil {
			t.Errorf("expected items in YAML, got %#v", doc)
		}
	})

	t.Run("table", func(t *testing.T) {
		stdout, stderr, code := run(t, "table")
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if !strings.Contains(stdout, "SLUG") || !strings.Contains(stdout, "curry") {
			t.Errorf("expected a table with headers and rows, got:\n%s", stdout)
		}
	})
}

// ---------------------------------------------------------------------------
// A.4 — --no-input never hangs: a prompt becomes a usage/confirmation failure.
// ---------------------------------------------------------------------------

func TestNoInputFailsInsteadOfPrompting(t *testing.T) {
	t.Run("auth login without token", func(t *testing.T) {
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"auth", "login", "--url", "https://mealie.example.com",
			"--no-input", "--config", cfg, "--output", "json",
		}})
		if code != ExitUsage {
			t.Fatalf("exit code = %d, want %d (usage)\nstderr:\n%s", code, ExitUsage, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout must stay clean, got:\n%s", stdout)
		}
		pl, _ := decodeErrorEnvelope(t, stderr)
		if pl.Code != "usage" {
			t.Errorf("code = %q, want usage", pl.Code)
		}
	})

	t.Run("destructive delete without yes", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("no request must be made; confirmation should fail first (%s %s)", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}))
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		_, stderr, code := runCLI(t, cliRun{args: []string{
			"recipe", "delete", "curry", "--no-input",
			"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != ExitUsage {
			t.Fatalf("exit code = %d, want %d (usage)\nstderr:\n%s", code, ExitUsage, stderr)
		}
		pl, _ := decodeErrorEnvelope(t, stderr)
		if pl.Code != "confirmation_required" {
			t.Errorf("code = %q, want confirmation_required", pl.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// A.5 — --quiet silences Info but never the error envelope.
// ---------------------------------------------------------------------------

func TestQuietSuppressesInfoNotErrors(t *testing.T) {
	t.Run("quiet hides the Info line, data still flows", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodGet && r.URL.Path == "/api/recipes/curry":
				_, _ = w.Write([]byte(`{"id":"r1","slug":"curry","name":"Curry"}`))
			case r.Method == http.MethodPost && r.URL.Path == "/api/households/shopping/lists/l1/recipe":
				_, _ = w.Write([]byte(`{"id":"l1","name":"Weekly","listItems":[{"id":"i1","note":"2 onions"}]}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"shopping", "recipe", "add", "curry", "--list", "l1", "--quiet",
			"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != 0 {
			t.Fatalf("exit code = %d, want 0\nstderr:\n%s", code, stderr)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Errorf("--quiet must suppress the Info line; stderr was:\n%s", stderr)
		}
		var list map[string]any
		if err := json.Unmarshal([]byte(stdout), &list); err != nil {
			t.Fatalf("data must still flow on stdout: %v\n%s", err, stdout)
		}
	})

	t.Run("quiet still emits the error envelope", func(t *testing.T) {
		srv := statusServer(404, `{"detail":"nope"}`)
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{args: []string{
			"recipe", "get", "missing", "--quiet",
			"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
		}})
		if code != ExitNotFound {
			t.Fatalf("exit code = %d, want %d\nstderr:\n%s", code, ExitNotFound, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout must be empty, got:\n%s", stdout)
		}
		pl, _ := decodeErrorEnvelope(t, stderr)
		if pl.Code != "not_found" {
			t.Errorf("--quiet must never silence the error contract; code = %q", pl.Code)
		}
	})
}

// ---------------------------------------------------------------------------
// A.6 — the --yes confirm gate guards destructive commands.
// ---------------------------------------------------------------------------

func TestConfirmGate(t *testing.T) {
	var sawDelete bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete && r.URL.Path == "/api/recipes/curry" {
			sawDelete = true
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	// Without --yes on a non-TTY: refuse, no request made.
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"recipe", "delete", "curry",
		"--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != ExitUsage {
		t.Fatalf("without --yes: exit = %d, want %d\nstderr:\n%s", code, ExitUsage, stderr)
	}
	if sawDelete {
		t.Fatal("a DELETE must not be sent without confirmation")
	}
	pl, _ := decodeErrorEnvelope(t, stderr)
	if pl.Code != "confirmation_required" {
		t.Errorf("code = %q, want confirmation_required", pl.Code)
	}

	// With --yes: proceed, the DELETE is hit.
	cfg2 := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"recipe", "delete", "curry", "--yes",
		"--url", srv.URL, "--token", "tok", "--config", cfg2, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("with --yes: exit = %d, want 0\nstderr:\n%s", code, stderr)
	}
	if !sawDelete {
		t.Fatal("with --yes the DELETE must be sent")
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["status"] != "deleted" {
		t.Errorf("expected status=deleted, got %#v", doc)
	}
}

// ---------------------------------------------------------------------------
// A.7 — the env-driven automation path (no flags): precedence flags > env > profile.
// ---------------------------------------------------------------------------

func TestEnvDrivenAutomation(t *testing.T) {
	t.Run("MEALIE_URL/TOKEN/OUTPUT drive a flagless request", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "per_page": 1, "total": 0, "total_pages": 1, "items": []any{},
			})
		}))
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		stdout, stderr, code := runCLI(t, cliRun{
			args: []string{"recipe", "list"},
			env: map[string]string{
				"MEALIE_URL":    srv.URL,
				"MEALIE_TOKEN":  "env-token",
				"MEALIE_OUTPUT": "json",
				"MEALIE_CONFIG": cfg,
			},
		})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if gotAuth != "Bearer env-token" {
			t.Errorf("Authorization = %q, want Bearer env-token", gotAuth)
		}
		// MEALIE_OUTPUT=json must have selected JSON with no --output flag.
		var doc map[string]any
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("MEALIE_OUTPUT=json should produce JSON: %v\n%s", err, stdout)
		}
	})

	t.Run("token_env profile resolves the token from the environment", func(t *testing.T) {
		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "per_page": 1, "total": 0, "total_pages": 1, "items": []any{},
			})
		}))
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		profile := "current_profile: ci\nprofiles:\n  ci:\n    base_url: " + srv.URL + "\n    token_env: MEALIE_CI_TOKEN\n"
		if err := writeFileForTest(cfg, profile); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, code := runCLI(t, cliRun{
			args: []string{"recipe", "list"},
			env: map[string]string{
				"MEALIE_CONFIG":   cfg,
				"MEALIE_PROFILE":  "ci",
				"MEALIE_CI_TOKEN": "secret-from-env",
				"MEALIE_OUTPUT":   "json",
			},
		})
		if code != 0 || stderr != "" {
			t.Fatalf("code=%d stderr=%q", code, stderr)
		}
		if gotAuth != "Bearer secret-from-env" {
			t.Errorf("Authorization = %q, want Bearer secret-from-env (token_env)", gotAuth)
		}
		if !strings.Contains(stdout, "items") {
			t.Errorf("expected JSON data on stdout, got:\n%s", stdout)
		}
	})

	t.Run("a flag overrides the environment", func(t *testing.T) {
		srv := listServer()
		defer srv.Close()
		cfg := filepath.Join(t.TempDir(), "config.yaml")
		// MEALIE_OUTPUT=json, but --output table on the command line must win.
		stdout, _, code := runCLI(t, cliRun{
			args: []string{"recipe", "list", "--output", "table"},
			env: map[string]string{
				"MEALIE_URL":    srv.URL,
				"MEALIE_TOKEN":  "tok",
				"MEALIE_OUTPUT": "json",
				"MEALIE_CONFIG": cfg,
			},
		})
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if !strings.Contains(stdout, "SLUG") {
			t.Errorf("--output table must override MEALIE_OUTPUT=json; got:\n%s", stdout)
		}
		if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
			t.Errorf("flag override failed: stdout looks like JSON:\n%s", stdout)
		}
	})
}

// TestInvalidOutputFormatStillFails pins the run() fallback: when the requested
// --output format is itself invalid, no machine envelope can be chosen, but the
// command must still fail with a usage exit code and a human error on stderr
// rather than crashing or exiting 0.
func TestInvalidOutputFormatStillFails(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"recipe", "list", "--output", "bogus",
		"--url", "https://mealie.example.com", "--token", "tok", "--config", cfg,
	}})
	if code != ExitUsage {
		t.Fatalf("exit = %d, want %d (usage)\nstderr:\n%s", code, ExitUsage, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout must stay clean, got:\n%s", stdout)
	}
	if !strings.Contains(stderr, "unknown output format") {
		t.Errorf("expected a human error about the bad format, got:\n%s", stderr)
	}
}

// ---------------------------------------------------------------------------
// Process-level contract: exec the real binary and assert the genuine exit code,
// stdout and stderr. This is the backstop that run() alone cannot give — it
// proves cmd/mealie wiring, os.Args passing, real getenv/default-config, and the
// signal-derived context are all intact (Codex high #1).
// ---------------------------------------------------------------------------

func TestProcessContractSubprocess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping subprocess exec in -short mode")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/recipes":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"page": 1, "per_page": 1, "total": 1, "total_pages": 1,
				"items": []map[string]any{{"slug": "curry", "name": "Curry"}},
			})
		case r.URL.Path == "/api/recipes/missing":
			w.Header().Set("X-Request-Id", "req-sub")
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"detail":"not found"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := filepath.Join(t.TempDir(), "config.yaml")

	exec := func(args ...string) (string, string, int) {
		t.Helper()
		return execMealie(t, srv.URL, cfg, args...)
	}

	t.Run("success", func(t *testing.T) {
		stdout, stderr, code := exec("recipe", "list", "--output", "json")
		if code != 0 {
			t.Fatalf("exit = %d, want 0\nstderr:\n%s", code, stderr)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Errorf("stderr must be clean, got:\n%s", stderr)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
			t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
		}
	})

	t.Run("error", func(t *testing.T) {
		stdout, stderr, code := exec("recipe", "get", "missing", "--output", "json")
		if code != ExitNotFound {
			t.Fatalf("exit = %d, want %d\nstderr:\n%s", code, ExitNotFound, stderr)
		}
		if stdout != "" {
			t.Errorf("stdout must be empty on error, got:\n%s", stdout)
		}
		pl, _ := decodeErrorEnvelope(t, stderr)
		if pl.Code != "not_found" || pl.HTTPStatus != 404 {
			t.Errorf("unexpected envelope: %#v", pl)
		}
		if pl.RequestID != "req-sub" {
			t.Errorf("request_id = %q, want req-sub", pl.RequestID)
		}
	})
}

// execMealie re-execs the test binary in subprocess mode (TestMain dispatches to
// Main when subprocessEnv=1), reaching a real loopback httptest server via the
// documented environment variables. It returns stdout, stderr and the real
// process exit code.
func execMealie(t *testing.T, url, cfg string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(testBinary(t), args...)
	// A deliberately minimal environment: no inherited MEALIE_* can leak in, and
	// MEALIE_CONFIG points the subprocess at a temp file so it never reads the
	// real user config. GOCOVERDIR is set so that, when the suite runs under
	// -cover, the coverage-instrumented child writes its data there instead of
	// printing "warning: GOCOVERDIR not set" to stderr — which would otherwise
	// dirty the very stderr this test asserts on.
	cmd.Env = []string{
		subprocessEnv + "=1",
		"MEALIE_URL=" + url,
		"MEALIE_TOKEN=tok",
		"MEALIE_CONFIG=" + cfg,
		"GOCOVERDIR=" + t.TempDir(),
	}
	var out, errBuf strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			code = ee.ExitCode()
		} else {
			t.Fatalf("exec %v: %v", args, err)
		}
	}
	return out.String(), errBuf.String(), code
}
