# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/gloos/mealie-cli/compare/HEAD...HEAD
