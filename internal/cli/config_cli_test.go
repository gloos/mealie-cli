package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// seedConfig writes a two-profile config file and returns its path.
func seedConfig(t *testing.T) string {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	content := "current_profile: home\n" +
		"profiles:\n" +
		"  home:\n" +
		"    base_url: https://home.example.com\n" +
		"    token: home-token\n" +
		"  work:\n" +
		"    base_url: https://work.example.com\n" +
		"    token_env: WORK_TOKEN\n"
	if err := writeFileForTest(cfg, content); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigPath(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yaml")
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"config", "path", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var doc map[string]string
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["path"] != cfg {
		t.Errorf("path = %q, want %q", doc["path"], cfg)
	}
}

func TestConfigList(t *testing.T) {
	cfg := seedConfig(t)
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"config", "list", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var infos []profileInfo
	if err := json.Unmarshal([]byte(stdout), &infos); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if len(infos) != 2 {
		t.Fatalf("expected 2 profiles, got %d", len(infos))
	}
	// Profiles are sorted by name: home (current, inline token), work (token_env).
	byName := map[string]profileInfo{}
	for _, info := range infos {
		byName[info.Name] = info
	}
	if !byName["home"].Current || !byName["home"].HasToken {
		t.Errorf("home should be current with a token: %#v", byName["home"])
	}
	if byName["work"].Current {
		t.Errorf("work should not be current")
	}
	if !byName["work"].HasToken {
		t.Errorf("work should report a token via token_env: %#v", byName["work"])
	}
}

func TestConfigUse(t *testing.T) {
	cfg := seedConfig(t)
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"config", "use", "work", "--config", cfg, "--output", "json",
	}})
	if code != 0 {
		t.Fatalf("exit = %d\nstderr:\n%s", code, stderr)
	}
	var doc map[string]string
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["current_profile"] != "work" {
		t.Errorf("current_profile = %q, want work", doc["current_profile"])
	}
	// The change must be persisted.
	data, _ := os.ReadFile(cfg)
	if !strings.Contains(string(data), "current_profile: work") {
		t.Errorf("config not updated on disk:\n%s", data)
	}
}

func TestConfigUseUnknownProfile(t *testing.T) {
	cfg := seedConfig(t)
	_, stderr, code := runCLI(t, cliRun{args: []string{
		"config", "use", "ghost", "--config", cfg, "--output", "json",
	}})
	if code != ExitNotFound {
		t.Fatalf("exit = %d, want %d (not_found)\nstderr:\n%s", code, ExitNotFound, stderr)
	}
	pl, _ := decodeErrorEnvelope(t, stderr)
	if pl.Code != "not_found" {
		t.Errorf("code = %q, want not_found", pl.Code)
	}
}

func TestConfigView(t *testing.T) {
	cfg := seedConfig(t)
	stdout, stderr, code := runCLI(t, cliRun{args: []string{
		"config", "view", "--config", cfg, "--output", "json",
	}})
	if code != 0 || stderr != "" {
		t.Fatalf("code=%d stderr=%q", code, stderr)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if doc["profile"] != "home" || doc["base_url"] != "https://home.example.com" {
		t.Errorf("unexpected view: %#v", doc)
	}
	if doc["has_token"] != true {
		t.Errorf("expected has_token=true, got %#v", doc["has_token"])
	}
	if doc["config_path"] != cfg {
		t.Errorf("config_path = %v, want %q", doc["config_path"], cfg)
	}
}
