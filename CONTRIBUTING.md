# Contributing to Mealie CLI

Thanks for your interest in improving Mealie CLI! This project welcomes issues,
ideas and pull requests.

## Ways to contribute

- **Report bugs** or **request features** via
  [issues](https://github.com/gloos/mealie-cli/issues).
- **Improve docs** — even fixing a typo is a valued contribution.
- **Send pull requests** for bug fixes and features.

If you're planning a larger change, please open an issue first so we can agree on
the approach before you invest time.

## Development setup

You'll need [Go](https://go.dev/dl/) (see `go.mod` for the minimum version) and,
optionally, [Docker](https://www.docker.com/) for the live smoke tests.

```sh
git clone https://github.com/gloos/mealie-cli
cd mealie-cli
make all        # fmt-check + vet + test + build
./bin/mealie --help
```

Common tasks (`make help` shows them all):

| Command       | What it does                                         |
|---------------|------------------------------------------------------|
| `make build`  | build `./bin/mealie`                                 |
| `make test`   | run all tests                                        |
| `make cover`  | run tests with coverage                              |
| `make vet`    | run `go vet`                                          |
| `make fmt`    | format all Go files                                  |
| `make spec`   | refresh the pinned Mealie OpenAPI spec via Docker    |

## Project layout

```
cmd/mealie            entry point
internal/cli          Cobra commands, flags, rendering, exit-code mapping
internal/config       XDG profiles, env/flag precedence
internal/buildinfo    version metadata (set via ldflags)
pkg/core              reusable Mealie client + workflows + DTOs (public SDK)
pkg/output            json/ndjson/yaml/table rendering + error envelope
api/specs             pinned upstream OpenAPI spec (contract-test source)
docs/adr              architecture decision records
```

The golden rule: **commands parse input, call `pkg/core`, and render via
`pkg/output`.** Commands never talk HTTP directly. This keeps a single
implementation of each workflow that a future MCP server can reuse.

## Coding guidelines

- **Formatting:** run `gofmt` (`make fmt`). CI fails on unformatted code.
- **Keep the agent contract intact:** data on stdout, diagnostics on stderr,
  stable error envelope, documented exit codes. See [docs/agents.md](docs/agents.md).
- **Fail loudly:** no silent fallbacks or swallowed errors. Wrap errors with
  context (`fmt.Errorf("...: %w", err)`).
- **Match the surrounding style:** naming, comment density and idiom.
- **Add tests** for new behaviour. New API calls should be covered by a unit
  test (using `httptest`) and, where relevant, a check in the contract tests.

## Testing

- **Real servers, not mocks.** Tests drive commands against an in-process
  `httptest.Server` and assert the real round-trip — request method/path/body in,
  data/exit code out. There is no HTTP mocking layer to drift from reality.
- **The `runCLI` harness** (`internal/cli/harness_test.go`) is the backbone for
  command-level tests. It drives the real `run()` entry point — the same one
  `Main()` defers to — with buffered streams, an injectable `*http.Client` (use
  `dialToServer(srv)` for a non-loopback URL, or pass `nil` to hit the server's
  loopback address directly) and an injectable environment map, returning
  `(stdout, stderr, exitCode)`. Because it goes through `run()`, it exercises the
  genuine classify → error-envelope → exit-code path rather than a copy.

  ```go
  stdout, stderr, code := runCLI(t, cliRun{
      args: []string{"recipe", "get", "curry", "--output", "json"},
      env:  map[string]string{"MEALIE_URL": srv.URL, "MEALIE_TOKEN": "tok"},
  })
  ```

- **The agent contract is pinned end-to-end** in `contract_cli_test.go`: the
  `classify` exit-code table, the full error envelope per error class (whole
  stderr parsed as one JSON object), success in every format, the
  `--no-input`/`--quiet`/`--yes` boundaries, and the env-driven automation path.
  A subprocess smoke test re-execs the test binary as the real CLI to assert the
  genuine *process* exit code/stdout/stderr. If you change anything in that
  contract, expect to update these tests deliberately.
- **Golden files** (`internal/cli/testdata/golden/`) pin human-facing output (the
  renderers and the `schema` tree) so a regression shows up as a reviewable diff.
  Regenerate them after an intentional change with:

  ```sh
  MEALIE_UPDATE_GOLDEN=1 go test ./internal/cli/...
  ```

  Review the diff, then run the suite plainly to confirm it passes.
- **Coverage:** `make cover`, or
  `go test ./... -coverprofile=cov.out && go tool cover -func=cov.out`.

## Working with the Mealie API

API types in `pkg/core` are **hand-written against the pinned OpenAPI spec** at
`api/specs/mealie/<version>/openapi.json`. When you add or change an API call:

1. Verify the path, method, query params and request/response schema against the
   pinned spec (it is the source of truth).
2. Add or update an entry in `pkg/core/contract_test.go` so drift is caught
   automatically.

To target a newer Mealie release, run `make spec` (requires Docker), update the
version constants in `internal/buildinfo`, and adjust the client where the spec
changed.

## Commit messages & pull requests

- Use clear, imperative commit subjects (e.g. "Add recipe export command").
- Conventional Commit prefixes (`feat:`, `fix:`, `docs:`, `chore:`, `test:`) are
  appreciated and feed the release changelog, but not required.
- Before opening a PR, run `make all` and make sure it's clean.
- Describe **what** changed and **why**, and link any related issue.

## Releases

Releases are automated with [GoReleaser](https://goreleaser.com) on version tags
(`vX.Y.Z`). Maintainers cut releases; you don't need to touch versioning in a PR.

## Code of Conduct

By participating you agree to abide by our
[Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions will be licensed under the
project's [MIT License](LICENSE).
