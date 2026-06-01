package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestWarnInsecureTransport(t *testing.T) {
	cases := []struct {
		name    string
		baseURL string
		token   string
		wantMsg bool
	}{
		// The token value is deliberately distinctive so the "never echo the
		// token" assertion below is not fooled by the literal word "token" in the
		// warning message.
		{"http remote with token warns", "http://recipes.example.com", "s3cr3t-Zx9", true},
		{"http remote with port warns", "http://recipes.example.com:9000", "s3cr3t-Zx9", true},
		{"https remote is silent", "https://recipes.example.com", "s3cr3t-Zx9", false},
		{"http localhost is silent", "http://localhost:9000", "s3cr3t-Zx9", false},
		{"http 127.0.0.1 is silent", "http://127.0.0.1:9000", "s3cr3t-Zx9", false},
		{"http ::1 is silent", "http://[::1]:9000", "s3cr3t-Zx9", false},
		{"http remote without token is silent", "http://recipes.example.com", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var errOut bytes.Buffer
			f := &Factory{opts: &globalOptions{}, Err: &errOut}
			f.warnInsecureTransport(c.baseURL, c.token)

			got := errOut.String()
			if c.wantMsg && got == "" {
				t.Fatalf("expected an insecure-transport warning for %q, got none", c.baseURL)
			}
			if !c.wantMsg && got != "" {
				t.Fatalf("expected no warning for %q, got %q", c.baseURL, got)
			}
			if c.token != "" && strings.Contains(got, c.token) {
				t.Fatalf("warning must never echo the token; got %q", got)
			}
		})
	}
}
