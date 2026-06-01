# Driving Mealie CLI from agents & automation

Mealie CLI is designed to be controlled reliably by scripts, CI jobs and AI
agents, not just typed by hand. This guide documents the contract those callers
can depend on.

> Everything here also applies to ordinary shell scripts — "agent" just means
> "a non-human caller that needs predictable, machine-readable behaviour".

## The output contract

- **stdout is data only.** In `--output json|ndjson|yaml`, stdout contains
  nothing but the requested data — no banners, spinners, or log lines.
- **stderr is everything else.** Progress messages, confirmations and errors all
  go to stderr, so a parser can consume stdout unconditionally.
- **Pick a format explicitly.** Default output is human-oriented tables. For
  automation, always pass `--output json` (single document) or
  `--output ndjson` (one object per line — best for lists and streaming).

```sh
mealie recipe list --all --output ndjson | jq -r '.slug'
```

You can set the default once for a whole session:

```sh
export MEALIE_OUTPUT=json
```

## Non-interactive behaviour

| Flag         | Effect                                                            |
|--------------|-------------------------------------------------------------------|
| `--no-input` | Never prompt. If input would be required, fail with exit code 2.  |
| `--yes`/`-y` | Skip confirmation prompts for destructive commands.               |
| `--quiet`/`-q` | Suppress incidental messages on stderr (data is unaffected).    |

A destructive command (`delete`) run without a TTY on **both** stdin and stderr
will refuse to proceed unless `--yes` is given, rather than hanging or consuming
piped input. The refusal is exit code 2 with the error `code` `confirmation_required`:

```sh
mealie --no-input recipe delete old-recipe --yes
```

`--quiet` silences incidental `Info` messages on stderr but **never** the error
envelope: a failing command still emits the full JSON error to stderr so a parser
can rely on it unconditionally.

## Errors

In a machine output format, errors are written to **stderr** as a stable JSON
envelope:

```json
{
  "error": {
    "code": "validation",
    "message": "name: field required",
    "http_status": 422,
    "retryable": false,
    "request_id": "abc123",
    "hint": "check your token or run `mealie auth login`",
    "details": { "fields": [ { "location": "name", "message": "field required" } ] }
  }
}
```

Field reference:

| Field         | Meaning                                                        |
|---------------|----------------------------------------------------------------|
| `code`        | stable machine token (see below)                               |
| `message`     | single-line human description                                  |
| `http_status` | upstream Mealie status code, when the error came from the API  |
| `retryable`   | `true` if retrying may succeed (transient)                     |
| `request_id`  | server correlation id, when provided                           |
| `hint`        | actionable next step                                           |
| `details`     | structured extras, e.g. per-field validation errors            |

Error `code` values from the API and transport: `auth`, `forbidden`,
`not_found`, `conflict`, `validation`, `bad_request`, `rate_limited`,
`server_error`, `network`. CLI-side failures add `usage`,
`confirmation_required` (a destructive command refused without `--yes`), `config`
(no server/token configured) and `exists` (refusing to overwrite a file without
`--force`). Branch on the exit code for failure *class*; use `code` for the
specific reason.

## Exit codes

Branch on the exit code rather than parsing text:

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

```sh
if ! mealie --no-input recipe get "$slug" --output json >/tmp/recipe.json 2>/tmp/err.json; then
  case $? in
    4) echo "no such recipe: $slug" ;;
    3) echo "auth problem — refresh the token" ;;
    7) echo "server unreachable — will retry later" ;;
    *) jq -r '.error.message' </tmp/err.json ;;
  esac
fi
```

## Discovering the command surface

`mealie schema --output json` emits the entire command tree — every command,
alias, and flag with its type and default — so an agent can enumerate
capabilities without scraping `--help` text.

```sh
mealie schema --output json | jq '.commands[] | .name'
```

## Authentication for automation

Prefer a long-lived API token supplied via the environment, so nothing is
written to disk:

```sh
export MEALIE_URL=https://mealie.example.com
export MEALIE_TOKEN=...      # long-lived token from Mealie → Profile → API Tokens
mealie recipe list --output json
```

Or reference an env var from a saved profile with `token_env` (see the README's
configuration section).

## Determinism tips

- Pass `--order-by`/`--order` (where available) for stable ordering instead of
  relying on server defaults.
- Use `--all` to fetch every result, or `--page`/`--per-page` for explicit
  pagination. In JSON/YAML output, list results include a `pagination` object.
  `--all` walks the result set **client-side**, fetching pages in batches sized by
  `--per-page` (or recipe's `--limit`, default 100) until they are exhausted, so a
  large instance degrades gracefully rather than failing on one oversized
  response. Because it spans every page, `--all` cannot be combined with an
  explicit `--page` — doing so is a usage error (exit 2), as is a negative
  `--per-page`/`--limit`.
- Timestamps are passed through from Mealie unchanged.

## Building shopping lists and backups

- `mealie shopping recipe add <recipe-slug> --list <id>` pushes a recipe's
  ingredients onto a shopping list (the core meal-planning move). Resolve the
  slug yourself with `--recipe-id <uuid>` to skip the lookup, and `--scale N` to
  multiply quantities. The confirmation reports the list's **total** item count
  ("now N items"), not just what was added.
- `mealie recipe export <slug>…` writes raw, lossless recipe JSON (every field the
  server sends, unlike the curated `recipe get`). With no `-O` a single recipe
  goes to stdout — composable in a pipeline. `-O <dir>` (a trailing slash or an
  existing directory) writes `<slug>.json` per recipe; `-O <file>` writes one
  recipe there. `--all` exports everything and requires `-O <dir>`. Existing files
  are never clobbered without `--force` (exit 5 otherwise). A multi-recipe export
  is all-or-nothing — every recipe is staged before any is published, so a mid-run
  failure leaves existing backups untouched — and files are written owner-only
  (`0600`) because the JSON is lossless and may carry private data. In a machine
  format, `recipe export` to file(s) emits `{"written":[…],"dir":"…"}` on stdout.

## A note on MCP

Mealie CLI deliberately keeps a clean, reusable core
([`pkg/core`](../pkg/core)) separate from the command layer. A Model Context
Protocol (MCP) server can wrap that same core without reimplementing any
workflows. If you're interested in an MCP adapter, see
[issue tracker](https://github.com/gloos/mealie-cli/issues) — contributions
welcome.
