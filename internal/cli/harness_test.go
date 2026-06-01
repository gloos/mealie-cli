package cli

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// subprocessEnv is the sentinel that flips the test binary into "be the CLI"
// mode: when set, TestMain dispatches straight to Main() and exits with the real
// process code, so a test can exec os.Args[0] and assert the genuine
// process-level contract (exit code + stdout + stderr) rather than an in-process
// approximation. See TestProcessContractSubprocess.
const subprocessEnv = "MEALIE_TEST_SUBPROCESS"

// TestMain lets the cli test binary double as the mealie binary. With
// subprocessEnv set it runs Main() and exits; otherwise it runs the normal test
// suite. This is the standard Go re-exec pattern (exec.Command(os.Args[0], …)).
func TestMain(m *testing.M) {
	if os.Getenv(subprocessEnv) == "1" {
		os.Exit(Main())
	}
	os.Exit(m.Run())
}

// cliRun configures a single in-process CLI invocation for runCLI.
type cliRun struct {
	// args is the full argument vector (e.g. {"recipe", "list", "--output", "json"}).
	args []string
	// client, when non-nil, is injected into every core client the factory builds,
	// so a command reaches a test server even with a non-loopback --url (which lets
	// the credential-transport guard be exercised). Use dialToServer(srv).
	client *http.Client
	// stdin is the data presented on the command's standard input.
	stdin string
	// env backs the injectable getenv: it models the process environment the
	// documented automation path reads (MEALIE_URL/TOKEN/OUTPUT/CONFIG/PROFILE,
	// NO_COLOR, and any token_env var). A nil map means an empty environment.
	env map[string]string
}

// runCLI drives the real run() — the same entry point Main() defers to — with
// buffered streams and an injectable environment, returning captured stdout,
// stderr and the process exit code. It is the backbone for the contract and
// command-layer tests: because it goes through run(), it exercises the genuine
// classify → EmitError → exit-code path rather than reimplementing it. It
// reuses dialToServer for the injected client.
func runCLI(t *testing.T, r cliRun) (stdout, stderr string, code int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	getenv := func(k string) string {
		if r.env == nil {
			return ""
		}
		return r.env[k]
	}
	f := &Factory{
		opts:       &globalOptions{},
		getenv:     getenv,
		In:         strings.NewReader(r.stdin),
		Out:        &out,
		Err:        &errBuf,
		httpClient: r.client,
	}
	code = run(context.Background(), f, r.args)
	return out.String(), errBuf.String(), code
}

// goldenDir is the root for golden fixtures used by assertGolden.
const goldenDir = "testdata/golden"

// assertGolden compares got against the golden file testdata/golden/<name>,
// rewriting it instead when MEALIE_UPDATE_GOLDEN=1 is set. Golden files keep
// human-facing output (renderers, the schema tree) under review: a deliberate
// change shows up as a reviewable diff, an accidental one as a failing test.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(goldenDir, name)
	if os.Getenv("MEALIE_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", name, err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (run with MEALIE_UPDATE_GOLDEN=1 to create it)", name, err)
	}
	if string(want) != got {
		t.Errorf("golden %s mismatch (run MEALIE_UPDATE_GOLDEN=1 to accept):\n--- want ---\n%s\n--- got ---\n%s", name, want, got)
	}
}

// writeFileForTest writes content to path, failing the test on error. Used to
// seed temp config files for the env-driven automation tests.
func writeFileForTest(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o600)
}

// testBinary returns the absolute path of the running test binary, which doubles
// as the mealie binary when re-execed with subprocessEnv set (see TestMain).
func testBinary(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	return exe
}
