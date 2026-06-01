package cli

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// dialToServer returns an http.Client that routes every request at srv
// regardless of the request's host, so a command driven with a non-loopback
// http:// URL still reaches the test server while the insecure-transport guard
// sees the public hostname. nil means "let core build its own client", which is
// used to exercise the real transport against the server's loopback address.
func dialToServer(srv *httptest.Server) *http.Client {
	addr := srv.Listener.Addr().String()
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, addr)
			},
		},
	}
}

// runRoot drives the root command with the given args against an injected HTTP
// client, returning captured stdout, stderr and the command error.
func runRoot(t *testing.T, client *http.Client, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errBuf bytes.Buffer
	f := &Factory{
		opts:       &globalOptions{},
		getenv:     func(string) string { return "" },
		In:         strings.NewReader(stdin),
		Out:        &out,
		Err:        &errBuf,
		httpClient: client,
	}
	root := NewRootCommand(f)
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return out.String(), errBuf.String(), err
}

// TestAuthLoginTokenFlowWarnsOnInsecureHTTP covers the --token login flow: the
// supplied token is validated with Whoami, and the credential guard must fire
// before that request when the server is reached over plaintext http to a
// non-loopback host.
func TestAuthLoginTokenFlowWarnsOnInsecureHTTP(t *testing.T) {
	const token = "tok-Zx9-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users/self" && r.Header.Get("Authorization") == "Bearer "+token {
			_, _ = w.Write([]byte(`{"username":"me","admin":false}`))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, err := runRoot(t, dialToServer(srv), "",
		"auth", "login",
		"--url", "http://recipes.example.com",
		"--token", token,
		"--no-input",
		"--config", cfg,
	)
	if err != nil {
		t.Fatalf("auth login failed: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "login credentials over an unencrypted http connection to recipes.example.com") {
		t.Fatalf("expected insecure-credentials warning on the token flow; got stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, token) {
		t.Fatalf("the warning must never echo the token; got stderr:\n%s", stderr)
	}
}

// TestAuthLoginPasswordFlowWarnsOnInsecureHTTP covers the --username/--password
// flow: the password is exchanged for an access token, then a long-lived token
// is minted, then Whoami runs — three credential-bearing requests. The single
// guard after URL normalisation must warn before any of them.
func TestAuthLoginPasswordFlowWarnsOnInsecureHTTP(t *testing.T) {
	const password = "pw-Zx9-secret"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/token":
			_ = r.ParseForm()
			if r.Form.Get("username") != "me" || r.Form.Get("password") != password {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"access_token":"acc"}`))
		case "/api/users/api-tokens":
			if r.Header.Get("Authorization") != "Bearer acc" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"token":"long-lived","name":"mealie-cli","id":"1"}`))
		case "/api/users/self":
			_, _ = w.Write([]byte(`{"username":"me","admin":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, err := runRoot(t, dialToServer(srv), "",
		"auth", "login",
		"--url", "http://recipes.example.com",
		"--username", "me",
		"--password", password,
		"--no-input",
		"--config", cfg,
	)
	if err != nil {
		t.Fatalf("auth login failed: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "login credentials over an unencrypted http connection to recipes.example.com") {
		t.Fatalf("expected insecure-credentials warning on the password flow; got stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, password) {
		t.Fatalf("the warning must never echo the password; got stderr:\n%s", stderr)
	}
}

// TestDoctorWarnsOnInsecureHTTP covers doctor's authenticated check: the public
// About probe must stay silent, and the token must trigger the guard only when
// the Whoami request is about to send it over plaintext http to a non-loopback
// host.
func TestDoctorWarnsOnInsecureHTTP(t *testing.T) {
	const token = "tok-Zx9-secret"
	var aboutAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/app/about":
			aboutAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"version":"v3.19.2","production":true}`))
		case "/api/users/self":
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"username":"me","admin":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, err := runRoot(t, dialToServer(srv), "",
		"doctor",
		"--url", "http://recipes.example.com",
		"--token", token,
		"--config", cfg,
	)
	if err != nil {
		t.Fatalf("doctor failed: %v\nstderr:\n%s", err, stderr)
	}
	// The public About probe must not carry the token: core attaches it to every
	// request, so doctor has to use a tokenless client for it or the token leaks
	// over plaintext http on the very first request, before the warning fires.
	if aboutAuth != "" {
		t.Fatalf("doctor's public About probe must not send the token; got Authorization %q", aboutAuth)
	}
	if !strings.Contains(stderr, "API token over an unencrypted http connection to recipes.example.com") {
		t.Fatalf("expected insecure-transport warning on the doctor auth check; got stderr:\n%s", stderr)
	}
	if strings.Contains(stderr, token) {
		t.Fatalf("the warning must never echo the token; got stderr:\n%s", stderr)
	}
}

// TestDoctorSilentOnLoopbackHTTP is the negative counterpart: with a real round
// trip to the server's own loopback address (127.0.0.1), the guard must stay
// silent even though the scheme is http. This also proves the guard is wired on
// the real transport path, not just when a client is injected.
func TestDoctorSilentOnLoopbackHTTP(t *testing.T) {
	const token = "tok-Zx9-secret"
	var aboutAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/app/about":
			aboutAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"version":"v3.19.2","production":true}`))
		case "/api/users/self":
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"username":"me","admin":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cfg := filepath.Join(t.TempDir(), "config.yaml")
	_, stderr, err := runRoot(t, nil, "",
		"doctor",
		"--url", srv.URL, // http://127.0.0.1:<port>
		"--token", token,
		"--config", cfg,
	)
	if err != nil {
		t.Fatalf("doctor failed: %v\nstderr:\n%s", err, stderr)
	}
	if aboutAuth != "" {
		t.Fatalf("doctor's public About probe must not send the token; got Authorization %q", aboutAuth)
	}
	if strings.Contains(stderr, "unencrypted http connection") {
		t.Fatalf("loopback http must not warn; got stderr:\n%s", stderr)
	}
}
