// Package core is the reusable Mealie client and workflow layer that sits
// beneath the CLI. It exposes a small, stable, hand-crafted API surface (clean
// public DTOs in this package) over the Mealie HTTP API, deliberately hiding
// upstream quirks such as the camelCase-query / snake_case-pagination mismatch
// and the households-vs-top-level path split.
//
// The CLI commands and any future MCP server are intended to call this package
// rather than the HTTP API directly, so there is a single implementation of each
// workflow. The bundled OpenAPI spec under api/specs is the source of truth for
// the contract tests that guard the DTO field names against upstream drift.
package core
