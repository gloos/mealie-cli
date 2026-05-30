package config

import "testing"

func TestNormaliseBaseURL(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"mealie.example.com", "https://mealie.example.com", false},
		{"https://mealie.example.com/", "https://mealie.example.com", false},
		{"https://mealie.example.com/api", "https://mealie.example.com", false},
		{"http://localhost:9000", "http://localhost:9000", false},
		{"  https://x.test  ", "https://x.test", false},
		{"", "", false},
		{"ftp://nope", "", true},
		{"https://", "", true},
	}
	for _, c := range cases {
		got, err := NormaliseBaseURL(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("NormaliseBaseURL(%q): expected error, got %q", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("NormaliseBaseURL(%q): unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("NormaliseBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolvePrecedence(t *testing.T) {
	cfg := &Config{
		CurrentProfile: "home",
		Profiles: map[string]*Profile{
			"home": {BaseURL: "https://file.test", Token: "file-token"},
		},
	}
	env := map[string]string{EnvURL: "https://env.test", EnvToken: "env-token"}
	getenv := func(k string) string { return env[k] }

	// env beats file
	res, err := cfg.Resolve(Overrides{}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if res.BaseURL != "https://env.test" || res.Token != "env-token" {
		t.Fatalf("env precedence failed: %+v", res)
	}

	// flags beat env
	res, err = cfg.Resolve(Overrides{BaseURL: "https://flag.test", Token: "flag-token"}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if res.BaseURL != "https://flag.test" || res.Token != "flag-token" {
		t.Fatalf("flag precedence failed: %+v", res)
	}
}

func TestResolveTokenEnvIndirection(t *testing.T) {
	cfg := &Config{
		Profiles: map[string]*Profile{
			DefaultProfileName: {BaseURL: "https://x.test", TokenEnv: "MY_TOKEN_VAR"},
		},
	}
	getenv := func(k string) string {
		if k == "MY_TOKEN_VAR" {
			return "from-env-var"
		}
		return ""
	}
	res, err := cfg.Resolve(Overrides{}, getenv)
	if err != nil {
		t.Fatal(err)
	}
	if res.Token != "from-env-var" {
		t.Fatalf("token_env indirection failed: %q", res.Token)
	}
	if res.Profile != DefaultProfileName {
		t.Fatalf("expected default profile, got %q", res.Profile)
	}
}
