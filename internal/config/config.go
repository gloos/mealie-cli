// Package config loads and resolves the CLI's connection settings. It supports
// multiple named profiles (one per Mealie server) and a strict precedence:
//
//	flag override  >  environment variable  >  profile file  >  built-in default
//
// The token may be stored inline (file mode 0600) or referenced indirectly via
// an environment variable name (token_env), which is the recommended pattern for
// CI and agents so secrets never touch disk.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultProfileName is used when no profile is selected anywhere.
const DefaultProfileName = "default"

// Environment variable names honoured during resolution.
const (
	EnvProfile = "MEALIE_PROFILE"
	EnvURL     = "MEALIE_URL"
	EnvToken   = "MEALIE_TOKEN"
	EnvConfig  = "MEALIE_CONFIG" // overrides the config file location
)

// Config is the on-disk configuration document.
type Config struct {
	CurrentProfile string              `yaml:"current_profile"`
	Profiles       map[string]*Profile `yaml:"profiles"`
}

// Profile holds the settings for a single Mealie server.
type Profile struct {
	// BaseURL is the server root, e.g. https://mealie.example.com (no /api suffix).
	BaseURL string `yaml:"base_url"`
	// Token is a long-lived Mealie API token stored inline. Optional; prefer TokenEnv.
	Token string `yaml:"token,omitempty"`
	// TokenEnv names an environment variable that holds the token at runtime.
	TokenEnv string `yaml:"token_env,omitempty"`
}

// Overrides are values supplied by command-line flags. Empty fields are ignored.
type Overrides struct {
	Profile string
	BaseURL string
	Token   string
}

// Resolved is the effective connection after applying all precedence rules.
type Resolved struct {
	Profile string
	BaseURL string
	Token   string
}

// Getenv mirrors os.Getenv; injectable for tests.
type Getenv func(string) string

// DefaultPath returns the XDG-compliant config file path, honouring MEALIE_CONFIG
// and XDG_CONFIG_HOME, falling back to ~/.config/mealie/config.yaml.
func DefaultPath(getenv Getenv) (string, error) {
	if getenv == nil {
		getenv = os.Getenv
	}
	if p := getenv(EnvConfig); p != "" {
		return p, nil
	}
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("locate home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "mealie", "config.yaml"), nil
}

// Load reads the config file. A missing file yields an empty, valid Config so
// first-run commands work without prior setup.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Config{Profiles: map[string]*Profile{}}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]*Profile{}
	}
	return &cfg, nil
}

// Save writes the config atomically with restrictive permissions (dir 0700,
// file 0600), since it may contain a token. It writes to a unique temp file in
// the same directory, fsyncs it, renames it into place, and cleans up the temp
// file on any failure so a half-written secret is never left behind.
func Save(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("secure temp config: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("write config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("flush config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close config: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

// Resolve applies the precedence rules and returns the effective connection.
// It does not require a token (commands such as `auth login` supply their own),
// but it does require a base URL once any profile work is attempted.
func (c *Config) Resolve(ov Overrides, getenv Getenv) (Resolved, error) {
	if getenv == nil {
		getenv = os.Getenv
	}

	profile := firstNonEmpty(ov.Profile, getenv(EnvProfile), c.CurrentProfile, DefaultProfileName)

	var p *Profile
	if c.Profiles != nil {
		p = c.Profiles[profile]
	}

	baseURL := firstNonEmpty(ov.BaseURL, getenv(EnvURL), profileField(p, func(p *Profile) string { return p.BaseURL }))

	token := firstNonEmpty(ov.Token, getenv(EnvToken))
	if token == "" && p != nil {
		if p.TokenEnv != "" {
			token = getenv(p.TokenEnv)
		}
		if token == "" {
			token = p.Token
		}
	}

	normalised, err := NormaliseBaseURL(baseURL)
	if err != nil {
		return Resolved{}, err
	}

	return Resolved{Profile: profile, BaseURL: normalised, Token: token}, nil
}

// NormaliseBaseURL validates the URL, defaults the scheme to https, strips a
// trailing slash and a redundant /api suffix (the client adds /api itself).
// An empty input returns an empty string without error, so callers can decide
// whether a URL is required for the command at hand.
func NormaliseBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %q: %w", raw, err)
	}
	if u.Host == "" {
		return "", fmt.Errorf("invalid base URL %q: missing host", raw)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("invalid base URL %q: scheme must be http or https", raw)
	}
	u.Path = strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api")
	u.RawQuery = ""
	u.Fragment = ""
	return strings.TrimRight(u.String(), "/"), nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func profileField(p *Profile, f func(*Profile) string) string {
	if p == nil {
		return ""
	}
	return f(p)
}
