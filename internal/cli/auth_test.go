package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users/self" && r.Header.Get("Authorization") == "Bearer tok" {
			_, _ = w.Write([]byte(`{"username":"chef","email":"chef@example.com","admin":true}`))
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"auth", "status", "--url", srv.URL, "--token", "tok", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["username"] != "chef" || doc["admin"] != true {
		t.Errorf("unexpected status: %#v", doc)
	}
}

func TestAuthStatusUnauthorized(t *testing.T) {
	srv := statusServer(401, `{"detail":"Could not validate credentials"}`)
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"auth", "status", "--url", srv.URL, "--token", "bad", "--config", cfg, "--output", "json",
	}})
	if code != ExitConfig {
		t.Fatalf("exit = %d, want %d (config/auth)\nstderr:\n%s", code, ExitConfig, stderr)
	}
	if stdout != "" {
		t.Errorf("stdout must be empty on error, got:\n%s", stdout)
	}
	pl, _ := decodeErrorEnvelope(t, stderr)
	if pl.Code != "auth" {
		t.Errorf("code = %q, want auth", pl.Code)
	}
}

func TestAuthLogout(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	seed := "current_profile: default\nprofiles:\n  default:\n    base_url: https://mealie.example.com\n    token: secret-token\n"
	if err := writeFileForTest(cfg, seed); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"auth", "logout", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["status"] != "logged_out" {
		t.Errorf("expected status=logged_out, got %#v", doc)
	}
	// The stored token must be gone from the file.
	data, _ := os.ReadFile(cfg)
	if strings.Contains(string(data), "secret-token") {
		t.Errorf("logout must remove the stored token; config still has it:\n%s", data)
	}
}
