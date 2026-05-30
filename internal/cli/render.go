package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

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
