<div align="center">

# Mealie CLI

**A fast, scriptable command-line client for [Mealie](https://github.com/mealie-recipes/mealie) — the self-hosted recipe manager and meal planner.**

[![CI](https://github.com/gloos/mealie-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/gloos/mealie-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/gloos/mealie-cli.svg)](https://pkg.go.dev/github.com/gloos/mealie-cli)
[![Go Report Card](https://goreportcard.com/badge/github.com/gloos/mealie-cli)](https://goreportcard.com/report/github.com/gloos/mealie-cli)
[![Release](https://img.shields.io/github/v/release/gloos/mealie-cli?sort=semver)](https://github.com/gloos/mealie-cli/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

[Install](#install) · [Quick start](#quick-start) · [Commands](#command-reference) · [Scripting](#scripting--automation) · [Contributing](CONTRIBUTING.md)

</div>

---

Mealie CLI brings your recipes, meal plans and shopping lists to the terminal.
It is a single static binary with no runtime to install, sensible defaults, and
clean output that's a pleasure to use by hand **and** trivial to drive from
scripts.

```console
$ mealie recipe list --search curry
SLUG                  NAME                  CATEGORIES
thai-green-curry      Thai Green Curry      Dinner, Thai
chicken-tikka-masala  Chicken Tikka Masala  Dinner, Indian

$ mealie mealplan add --date 2026-06-02 --type dinner --recipe thai-green-curry
Added dinner entry for 2026-06-02 (id 41)
```

> [!NOTE]
> **Unofficial, community project.** Mealie CLI is not affiliated with or endorsed
> by the Mealie project or its maintainers. "Mealie" is the name of the upstream
> server this tool talks to.

## Features

- 🍳 **Recipe-centric** — search, view, create, import-from-URL, and delete recipes.
- 📅 **Meal planning** — list, inspect today's plan, and schedule recipes or notes.
- 🛒 **Shopping lists** — create lists, add items, check things off as you shop.
- 📦 **Single binary** — written in Go; nothing else to install.
- 🔌 **Multiple servers** — named profiles with env-var and flag overrides.
- 🤖 **Scripting & automation friendly** — structured `--output json|ndjson|yaml`,
  data on stdout, diagnostics on stderr, stable exit codes, and a
  machine-readable error format. See [Scripting & automation](#scripting--automation).
- 📚 **Reusable Go SDK** — the same client the CLI uses lives in
  [`pkg/core`](pkg/core) for your own Go programs.

## Install

### Prebuilt binaries

Download an archive for your platform from the
[Releases](https://github.com/gloos/mealie-cli/releases) page, extract it, and
put the `mealie` binary on your `PATH`.

#### Verify the download (optional but recommended)

Each release ships a `checksums.txt`, signed keyless with
[cosign](https://docs.sigstore.dev/) via GitHub's OIDC identity, so you can
confirm it came from this repo's tagged release workflow. Releases after v0.2.0
ship the signature as a single Sigstore bundle, `checksums.txt.sigstore.json`
(verifying it needs cosign v3+). Verification is two steps: first confirm the
signature on `checksums.txt`, then confirm your downloaded archive is listed in
that now-trusted file. Download `checksums.txt` and `checksums.txt.sigstore.json`
alongside your archive, then:

```sh
# 1. Verify checksums.txt was signed by this repo's tagged release workflow.
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/gloos/mealie-cli/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

# 2. Verify your archive's digest against the trusted checksums.txt.
#    --ignore-missing only checks the archive(s) you actually downloaded.
sha256sum --ignore-missing -c checksums.txt      # Linux
shasum -a 256 --ignore-missing -c checksums.txt  # macOS
```

Step 1 on its own does not verify any archive — both steps are required.

v0.2.0 predates the bundle change and instead ships `checksums.txt.pem` and
`checksums.txt.sig`; to verify it, replace step 1 with (works on cosign v2 or
v3):

```sh
cosign verify-blob \
  --certificate checksums.txt.pem \
  --signature checksums.txt.sig \
  --certificate-identity-regexp '^https://github\.com/gloos/mealie-cli/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

### Go

```sh
go install github.com/gloos/mealie-cli/cmd/mealie@latest
```

> Homebrew distribution is planned for a future release.

Verify the install:

```sh
mealie version
```

## Quick start

You'll need a Mealie server and an API token. Generate a long-lived token in
Mealie under **Profile → API Tokens**.

```sh
# 1. Log in (saves a profile to ~/.config/mealie/config.yaml, mode 0600)
mealie auth login --url https://mealie.example.com --token "$MY_TOKEN"

# 2. Check everything is wired up
mealie doctor

# 3. Cook
mealie recipe import https://www.example.com/recipes/lasagne
mealie recipe list --search lasagne
mealie mealplan add --type dinner --recipe lasagne
mealie shopping create "Weekly shop"
mealie shopping item add --list <list-id> "500g beef mince"
```

No token yet? Log in with your username and password and Mealie CLI will mint a
long-lived token for you and store it:

```sh
mealie auth login --url https://mealie.example.com --username you
```

## Configuration

Settings resolve with a clear precedence:

```
flags  >  environment variables  >  profile file  >  built-in defaults
```

| Environment variable | Purpose                                   |
|----------------------|-------------------------------------------|
| `MEALIE_URL`         | server base URL                           |
| `MEALIE_TOKEN`       | API token                                 |
| `MEALIE_PROFILE`     | profile to use                            |
| `MEALIE_OUTPUT`      | default output format                     |
| `MEALIE_CONFIG`      | config file path override                 |

The config file lives at `$XDG_CONFIG_HOME/mealie/config.yaml` (default
`~/.config/mealie/config.yaml`) and is written with `0600` permissions because
it may contain a token:

```yaml
current_profile: home
profiles:
  home:
    base_url: https://mealie.example.com
    token: <long-lived-token>     # stored inline (file is 0600)
  ci:
    base_url: https://mealie.example.com
    token_env: MEALIE_TOKEN_CI    # read from this env var at runtime — nothing on disk
```

Use `token_env` for CI and automation so the secret never touches disk. Switch
servers with profiles:

```sh
mealie --profile ci recipe list
mealie config list
mealie config use home
```

## Command reference

```
mealie auth      login | status | logout      Authenticate against a server
mealie config    path | list | use | view     Manage profiles & settings
mealie doctor                                  Diagnose connectivity & compatibility
mealie version                                 Show version & compatibility info
mealie schema                                  Emit the command tree as JSON
mealie recipe    list | get | create | import | delete
mealie mealplan  list | today | add | delete
mealie shopping  list | get | create | delete
mealie shopping item  add | check | uncheck | delete
mealie completion  bash | zsh | fish | powershell
```

Run `mealie <command> --help` for full flags and examples. A few highlights:

```sh
mealie recipe list --tag vegetarian --tag quick --all
mealie recipe get thai-green-curry
mealie mealplan list --start 2026-06-01 --end 2026-06-07
mealie shopping get <list-id>
mealie shopping item check <item-id>
```

## Scripting & automation

Every read command can emit structured output, and Mealie CLI follows a strict
contract so tools and agents can drive it reliably:

- **stdout carries only data**; progress, prompts and errors go to **stderr**.
- `--output json` emits one document; `--output ndjson` emits one object per line
  (lists stream their items, ideal for `jq` and pipelines).
- **Errors in machine formats** are emitted to stderr as a stable envelope:
  ```json
  {"error":{"code":"not_found","message":"recipe not found","http_status":404,"retryable":false}}
  ```
- `--no-input` never prompts (fails instead), `--yes` confirms destructive
  actions, and `--quiet` silences incidental stderr messages.
- `mealie schema --output json` describes every command and flag for discovery.

```sh
# Slugs of every vegetarian recipe, newline-delimited
mealie recipe list --tag vegetarian --all --output ndjson | jq -r '.slug'

# Fail fast in a script, no prompts
mealie --no-input recipe delete old-recipe --yes
```

### Exit codes

| Code | Meaning                                   |
|-----:|-------------------------------------------|
| 0    | success                                   |
| 1    | generic / unexpected error                |
| 2    | usage error (bad flags or arguments)      |
| 3    | configuration or authentication problem   |
| 4    | resource not found                        |
| 5    | conflict (e.g. already exists)            |
| 6    | request rejected by server validation     |
| 7    | network failure / transient server error  |

Driving Mealie CLI from an AI agent or other tooling? See the
[agent integration guide](docs/agents.md).

## Compatibility

Mealie CLI targets **Mealie v3.x** (the households data model). The API types
are **hand-written against a pinned OpenAPI spec**
([`api/specs/mealie`](api/specs/mealie)) and guarded by contract tests, so
upstream changes are caught early. `mealie doctor` checks the live server version
against the supported minimum.

## Using the Go SDK

The client that powers the CLI is a standalone, importable package:

```go
import "github.com/gloos/mealie-cli/pkg/core"

client, _ := core.New("https://mealie.example.com", token)
recipes, _ := client.ListRecipes(ctx, core.RecipeListOptions{
    ListOptions: core.ListOptions{Search: "curry"},
})
```

See the [reference docs](https://pkg.go.dev/github.com/gloos/mealie-cli/pkg/core).

## Development

```sh
make build      # build ./bin/mealie
make test       # run tests
make all        # fmt-check + vet + test + build
make spec       # refresh the pinned OpenAPI spec from a Docker container
```

Architecture and the rationale behind the big technical choices are recorded as
[ADRs](docs/adr). Contributions are very welcome — start with
[CONTRIBUTING.md](CONTRIBUTING.md).

## Acknowledgements

Built on top of the excellent [Mealie](https://github.com/mealie-recipes/mealie)
project. Thanks to its maintainers and community.

## License

[MIT](LICENSE) © Mealie CLI contributors
