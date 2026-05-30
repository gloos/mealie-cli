package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "config.yaml")
	cfg := &Config{
		CurrentProfile: "home",
		Profiles: map[string]*Profile{
			"home": {BaseURL: "https://x.test", Token: "secret"},
		},
	}
	if err := Save(path, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentProfile != "home" || got.Profiles["home"].Token != "secret" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestSaveFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permissions not applicable on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := Save(path, &Config{Profiles: map[string]*Profile{}}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("config file perms = %o, want 600 (it may hold a token)", perm)
	}
}

func TestSaveLeavesNoTempFileOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := Save(path, &Config{Profiles: map[string]*Profile{}}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "config.yaml" {
			t.Errorf("unexpected leftover file in config dir: %s", e.Name())
		}
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if cfg.Profiles == nil {
		t.Fatal("expected non-nil Profiles map for missing file")
	}
}

func TestDefaultPathHonoursXDG(t *testing.T) {
	getenv := func(k string) string {
		switch k {
		case "XDG_CONFIG_HOME":
			return "/tmp/xdg"
		default:
			return ""
		}
	}
	path, err := DefaultPath(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/tmp/xdg/mealie/config.yaml" {
		t.Fatalf("DefaultPath = %q", path)
	}
}

func TestDefaultPathHonoursMealieConfig(t *testing.T) {
	getenv := func(k string) string {
		if k == EnvConfig {
			return "/custom/path.yaml"
		}
		return ""
	}
	path, err := DefaultPath(getenv)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/custom/path.yaml" {
		t.Fatalf("DefaultPath = %q", path)
	}
}
