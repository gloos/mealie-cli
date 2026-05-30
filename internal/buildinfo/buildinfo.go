// Package buildinfo exposes version metadata stamped into the binary at build
// time via -ldflags (see .goreleaser.yaml and the Makefile).
package buildinfo

var (
	// Version is the released semantic version, e.g. "1.2.0". "dev" for local builds.
	Version = "dev"
	// Commit is the git commit the binary was built from.
	Commit = "none"
	// Date is the RFC3339 build timestamp.
	Date = "unknown"
)

// TestedMealieVersion records the upstream Mealie release whose OpenAPI spec the
// bundled client was generated from. Surfaced by `mealie version` and used by
// `mealie doctor` to warn on incompatible servers.
const TestedMealieVersion = "v3.19.2"

// MinMealieVersion is the lowest Mealie server version this CLI supports. Mealie
// v3 introduced the households model that the command tree depends on.
const MinMealieVersion = "v3.0.0"
