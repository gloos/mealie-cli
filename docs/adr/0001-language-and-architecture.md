# ADR 0001 — Language, API client and architecture

- Status: accepted
- Date: 2026-05-30
- Deciders: Claude (Opus 4.8) + Codex (independent consensus), with product sign-off from the maintainer
- Review: Codex adversarial code review (2026-05-31) confirmed the approach; fixes applied to API correctness, client robustness, config security, and the agent contract before first release.

## Context

We are building a top-class, open-source CLI for [Mealie](https://github.com/mealie-recipes/mealie)
that must be excellent for **both humans and AI agents**, and is itself built and
maintained largely by agents. The first release targets the **meal-planning
workflow**: recipes, meal plans and shopping lists.

Mealie is a FastAPI application exposing an OpenAPI **3.1** spec at
`/openapi.json` (175 paths, 243 schemas at v3.19.2), with long-lived API-token
auth (`Authorization: Bearer …`). Meal plans, shopping lists and cookbooks live
under `/api/households/…`; recipes/foods/units are top-level under `/api`.

## Decision

### Language: Go

Two independent reviews (Claude and Codex) converged on **Go**. The dominant,
compounding factor is distribution: a single static binary with no runtime is
the lowest-friction artefact for agents, CI, servers and non-Node users.
Go also has the strongest CLI ecosystem (Cobra, GoReleaser) and trivial
cross-compilation. The maintainer's house stack is TypeScript, but the project
is agent-maintained and the install/distribution win outweighs stack affinity.

### API client: hand-written typed core, codegen kept in reserve

We spiked [`ogen`](https://github.com/ogen-go/ogen) against the real v3.19.2
spec: it generated a compiling client (~42k LOC) and **proved OpenAPI 3.1
codegen is viable in Go** (unlike `oapi-codegen`, which does not yet support
3.1). 

For the **curated v1 surface** (~a dozen endpoints) we nonetheless chose a
**hand-written typed client** in [`pkg/core`](../../pkg/core):

- v1 is a deliberately narrow, curated surface, so the generated client's main
  benefit — breadth — is unused, while its cost (tens of thousands of committed
  lines and Opt*/nullable mapping) is real.
- A hand-written core yields clean, stable public DTOs — a genuinely usable Go
  SDK — instead of leaking generated wrapper types.
- Upstream **drift** (Codex's main concern with hand-writing) is mitigated by
  **contract tests** that assert our paths and DTO field names against the
  pinned spec, plus a `doctor` version guard.

The pinned spec stays committed under `api/specs/`, and `make spec` refreshes it.
If/when we expand toward full API coverage or an MCP server, ogen can generate
the broad client to sit beneath the same `pkg/core` facade.

### Architecture

```
cmd/mealie            entry point
internal/cli          Cobra commands, flags, rendering, exit-code mapping
internal/config       XDG profiles, env/flag precedence
internal/buildinfo    version metadata (ldflags)
pkg/core              reusable Mealie client + workflows + DTOs  (public SDK)
pkg/output            json/ndjson/yaml/table rendering + error envelope
api/specs             pinned upstream OpenAPI spec (contract-test source)
```

Commands parse input, call `pkg/core`, and render via `pkg/output`. They never
talk HTTP directly, so a future MCP server can wrap the same core without a
second implementation.

### Agent contract (first-class, not bolted on)

- `--output json|ndjson|yaml`; data on stdout, everything else on stderr.
- Stable error envelope `{"error": {code, message, http_status, retryable, …}}`.
- Documented exit codes (0–7) by failure class.
- `--no-input`, `--quiet`, `--yes`, stdin support, and `mealie schema` for
  programmatic command discovery.

### Config & auth

XDG YAML with multiple named profiles; precedence flags > env > file > default.
Token via inline file (0600) or a `token_env` indirection for CI/agents.

### Testing & distribution

- Unit tests + `httptest` for the client + **contract tests** against the spec.
- GoReleaser → GitHub Releases binaries, Homebrew tap, `go install`.

## Consequences

- We own a small amount of mapping code and must keep contract tests honest.
- Expanding to full coverage later means reintroducing codegen beneath `pkg/core`
  — a contained change, not a rewrite.
