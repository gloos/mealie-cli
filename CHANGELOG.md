# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Security

- Pin the documented `cosign verify-blob` identity to this repo's tagged release
  workflow (`…/.github/workflows/release.yml@refs/tags/v…`) instead of the
  broader `…/mealie-cli/.*`, so a signature minted by any other workflow in the
  repo can't satisfy the published verification command.

### Changed

- Release signing moved to cosign v3 (`sigstore/cosign-installer` bumped to
  v4.1.2). `checksums.txt` is now signed into a single Sigstore bundle,
  `checksums.txt.sigstore.json`, replacing the separate `checksums.txt.pem` and
  `checksums.txt.sig` files. Verify with `cosign verify-blob --bundle
  checksums.txt.sigstore.json …` (needs cosign v3+); the README has the full
  two-step flow.

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

[Unreleased]: https://github.com/gloos/mealie-cli/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/gloos/mealie-cli/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/gloos/mealie-cli/releases/tag/v0.1.0
