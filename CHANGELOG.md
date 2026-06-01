# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.1] - 2026-06-01

### Documentation

- Clarified the agent contract in [`docs/agents.md`](docs/agents.md): a
  destructive command refused without `--yes` fails exit 2 with the error `code`
  `confirmation_required`; `--quiet` silences incidental messages but never the
  error envelope; and the error `code` vocabulary now distinguishes API/transport
  codes from CLI-side ones (`usage`, `confirmation_required`, `config`, `exists`).
  No behaviour changed — these guarantees are now pinned by end-to-end contract
  tests.

### Internal

- The agent contract is now verified end-to-end: the `classify` exit-code table,
  the full JSON error envelope per error class, stdout/stderr separation in every
  output format, the `--no-input`/`--quiet`/`--yes` boundaries, the env-driven
  automation path, and the process-level exit code (via a subprocess smoke test).
  Human-facing renderers and the `schema` discovery tree are pinned with golden
  files. `Main()` gained a small behaviour-preserving `run()` seam so the contract
  is driven through the real code path rather than a copy.

## [0.3.0] - 2026-06-01

### Added

- `mealie shopping recipe add <recipe-slug> --list <id>` pushes a recipe's
  ingredients onto a shopping list via Mealie's non-deprecated bulk endpoint.
  `--scale N` multiplies quantities (an unset scale honours the server default of
  1); `--recipe-id <uuid>` skips the slug lookup. The confirmation reports the
  list's total item count.
- `mealie recipe export <slug>…` writes raw, lossless recipe JSON for backups —
  every field the server sends, unlike the curated `recipe get`. With no `-O` a
  single recipe goes to stdout; `-O <dir>` writes `<slug>.json` per recipe; `-O
  <file>` writes one recipe there; `--all` exports everything into a directory.
  A multi-recipe export is all-or-nothing — every recipe is staged before any is
  published, so a mid-run failure leaves existing backups untouched. Files are
  written owner-only (`0600`, like the config file) since recipe JSON is lossless
  and may carry private data, slug filenames are path-traversal guarded, and
  existing files are never overwritten without `--force`.

### Changed

- `--all` now paginates **client-side**: instead of asking the server for the
  whole result set in one unbounded response (the old `perPage=-1` sentinel,
  which could exceed the response-size cap on large instances), it walks the
  pages in batches sized by `--per-page`/`--limit` (default 100) until exhausted.
  Combining `--all` with an explicit `--page`, or passing a negative
  `--per-page`/`--limit`, is now a usage error rather than being silently
  ignored. The `pkg/core` SDK's `All()`/`perPage=-1` helper is unchanged.
- Release signing moved to cosign v3 (`sigstore/cosign-installer` bumped to
  v4.1.2). `checksums.txt` is now signed into a single Sigstore bundle,
  `checksums.txt.sigstore.json`, replacing the separate `checksums.txt.pem` and
  `checksums.txt.sig` files. Verify with `cosign verify-blob --bundle
  checksums.txt.sigstore.json …` (needs cosign v3+); the README has the full
  two-step flow.

### Security

- Pin the documented `cosign verify-blob` identity to this repo's tagged release
  workflow (`…/.github/workflows/release.yml@refs/tags/v…`) instead of the
  broader `…/mealie-cli/.*`, so a signature minted by any other workflow in the
  repo can't satisfy the published verification command.

## [0.2.0] - 2026-06-01

### Security

- Warn on stderr before an API token or login credentials are sent over an
  unencrypted `http://` connection to a non-loopback host. The check covers
  `auth login` (both the token and the username/password flows) and `doctor`,
  is not silenced by `--quiet`, and never echoes the secret. `doctor`'s public
  connectivity probe no longer sends the token at all.
- Cap buffered HTTP response bodies at 50 MiB so a hostile or malfunctioning
  server cannot exhaust client memory with an unbounded stream.

### Changed

- Pin all GitHub Actions to commit SHAs, and pin GoReleaser to `v2.16.0` rather
  than a floating `~> v2` range, so the release tooling is a reviewed input.

### Added

- Sign `checksums.txt` keyless with cosign via GitHub OIDC on release, and
  document the complete two-step verification (verify the signature on
  `checksums.txt`, then check the downloaded archive's digest against it) in the
  README and the GoReleaser config.

## [0.1.0] - 2026-05-31

### Added

- Initial release of Mealie CLI.
- `auth` commands: `login`, `status`, `logout` (long-lived API tokens, with
  username/password login that mints a token).
- `recipe` commands: `list`, `get`, `create`, `import` (from URL), `delete`.
- `mealplan` commands: `list`, `today`, `add`, `delete`.
- `shopping` commands: `list`, `get`, `create`, `delete`, and
  `shopping item add | check | uncheck | delete`.
- `config` commands: `path`, `list`, `use`, `view`; multi-profile support with
  XDG config and env/flag precedence.
- `doctor`, `version`, and `schema` (machine-readable command discovery).
- Agent/automation contract: `--output json|ndjson|yaml`, data on stdout,
  diagnostics on stderr, stable error envelope, documented exit codes,
  `--no-input`/`--quiet`/`--yes`.
- Reusable Go SDK in `pkg/core`, with contract tests against a pinned Mealie
  OpenAPI spec (v3.19.2).

[Unreleased]: https://github.com/gloos/mealie-cli/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/gloos/mealie-cli/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/gloos/mealie-cli/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gloos/mealie-cli/releases/tag/v0.1.0
