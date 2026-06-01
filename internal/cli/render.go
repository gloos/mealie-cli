package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

// newTable returns a tabwriter pre-seeded with a tab-separated header row.
func newTable(w io.Writer, headers ...string) *tabwriter.Writer {
	tw := output.NewTabWriter(w)
	fmt.Fprintln(tw, strings.Join(headers, "\t"))
	return tw
}

// emitPage renders a paginated result. In JSON/YAML it wraps the items with
// pagination metadata; in NDJSON and table modes it renders the items directly
// (the human view via the supplied callback).
func emitPage[T any](p *output.Printer, page *core.Page[T], human output.HumanFunc) error {
	switch p.Format {
	case output.FormatJSON, output.FormatYAML:
		return p.Emit(map[string]any{
			"items": page.Items,
			"pagination": map[string]int{
				"page":        page.Page,
				"per_page":    page.PerPage,
				"total":       page.Total,
				"total_pages": page.TotalPages,
			},
		}, human)
	default:
		return p.Emit(page.Items, human)
	}
}

// fetchAllPage drives core.FetchAll for a list command's --all flag and wraps
// the gathered items in a synthetic single page, so the result renders through
// the same emitPage path as an ordinary page. --page is meaningless with --all
// (which fetches every page) and is rejected when explicitly set; a negative
// batch size is a usage error rather than the re-sent perPage=-1 sentinel. The
// batch size comes from the existing --per-page/--limit flag, defaulting to
// FetchAll's bounded page size when left at 0.
func fetchAllPage[T any](cmd *cobra.Command, perPage int, fetch func(page, perPage int) (*core.Page[T], error)) (*core.Page[T], error) {
	if cmd.Flags().Changed("page") {
		return nil, usageError("--page cannot be combined with --all (which fetches every page)")
	}
	if perPage < 0 {
		return nil, usageError(fmt.Sprintf("--per-page must be a positive batch size, got %d", perPage))
	}
	items, err := core.FetchAll(cmd.Context(), perPage, fetch)
	if err != nil {
		return nil, err
	}
	return &core.Page[T]{Items: items, Page: 1, PerPage: len(items), Total: len(items), TotalPages: 1}, nil
}

// orgNames joins organizer names (categories/tags/tools) for table display.
func orgNames(refs []core.OrganizerRef) string {
	names := make([]string, 0, len(refs))
	for _, r := range refs {
		names = append(names, r.Name)
	}
	return strings.Join(names, ", ")
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
