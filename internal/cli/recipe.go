package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

func newRecipeCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "recipe",
		Aliases: []string{"recipes"},
		Short:   "Manage recipes",
	}
	cmd.AddCommand(
		newRecipeListCmd(f),
		newRecipeGetCmd(f),
		newRecipeCreateCmd(f),
		newRecipeImportCmd(f),
		newRecipeExportCmd(f),
		newRecipeDeleteCmd(f),
	)
	return cmd
}

func newRecipeListCmd(f *Factory) *cobra.Command {
	var (
		search             string
		page, perPage, lim int
		all                bool
		orderBy, order     string
		categories, tags   []string
		tools              []string
		cookbook           string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recipes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if perPage == 0 && lim > 0 {
				perPage = lim
			}
			opts := core.RecipeListOptions{
				ListOptions: core.ListOptions{
					Page:           page,
					PerPage:        perPage,
					OrderBy:        orderBy,
					OrderDirection: order,
					Search:         search,
				},
				Categories: categories,
				Tags:       tags,
				Tools:      tools,
				Cookbook:   cookbook,
			}
			var res *core.Page[core.RecipeSummary]
			if all {
				res, err = fetchAllPage(cmd, perPage, func(page, pp int) (*core.Page[core.RecipeSummary], error) {
					o := opts
					o.Page = page
					o.PerPage = pp
					return c.ListRecipes(ctx, o)
				})
			} else {
				res, err = c.ListRecipes(ctx, opts)
			}
			if err != nil {
				return err
			}
			return emitPage(p, res, func(w io.Writer) error {
				tw := newTable(w, "SLUG", "NAME", "CATEGORIES")
				for _, r := range res.Items {
					fmt.Fprintf(tw, "%s\t%s\t%s\n", r.Slug, r.Name, dash(orgNames(r.Categories)))
				}
				return tw.Flush()
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&search, "search", "s", "", "full-text search query")
	flags.IntVar(&page, "page", 0, "page number (1-based)")
	flags.IntVar(&perPage, "per-page", 0, "results per page")
	flags.IntVar(&lim, "limit", 0, "max results in one page (alias for --per-page; use --all to fetch everything)")
	flags.BoolVar(&all, "all", false, "fetch every result, paginating client-side in --per-page/--limit batches")
	flags.StringVar(&orderBy, "order-by", "", "field to order by (e.g. name, created_at)")
	flags.StringVar(&order, "order", "", "order direction: asc or desc")
	flags.StringSliceVar(&categories, "category", nil, "filter by category (repeatable)")
	flags.StringSliceVar(&tags, "tag", nil, "filter by tag (repeatable)")
	flags.StringSliceVar(&tools, "tool", nil, "filter by tool (repeatable)")
	flags.StringVar(&cookbook, "cookbook", "", "filter by cookbook slug")
	return cmd
}

func newRecipeGetCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <slug>",
		Short: "Show a recipe by slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			r, err := c.GetRecipe(ctx, args[0])
			if err != nil {
				return err
			}
			return p.Emit(r, func(w io.Writer) error {
				renderRecipe(w, r)
				return nil
			})
		},
	}
}

func newRecipeCreateCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new empty recipe",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			slug, err := c.CreateRecipe(ctx, args[0])
			if err != nil {
				return err
			}
			p.Info("Created recipe %q", slug)
			return p.Emit(map[string]string{"slug": slug}, func(w io.Writer) error {
				fmt.Fprintln(w, slug)
				return nil
			})
		},
	}
}

func newRecipeImportCmd(f *Factory) *cobra.Command {
	var includeTags bool
	cmd := &cobra.Command{
		Use:   "import <url>",
		Short: "Import a recipe by scraping a URL",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			slug, err := c.CreateRecipeFromURL(ctx, args[0], includeTags)
			if err != nil {
				return err
			}
			p.Info("Imported recipe %q from %s", slug, args[0])
			return p.Emit(map[string]string{"slug": slug}, func(w io.Writer) error {
				fmt.Fprintln(w, slug)
				return nil
			})
		},
	}
	cmd.Flags().BoolVar(&includeTags, "tags", true, "import tags from the source site")
	return cmd
}

func newRecipeExportCmd(f *Factory) *cobra.Command {
	var (
		out   string
		all   bool
		force bool
	)
	cmd := &cobra.Command{
		Use:   "export [slug...]",
		Short: "Export recipes as lossless JSON (for backup)",
		Long: "Export one or more recipes as raw, lossless JSON — every field the\n" +
			"server sends, unlike the curated view from `recipe get`.\n\n" +
			"Destinations:\n" +
			"  (no -O)         a single recipe is written to stdout\n" +
			"  -O <dir>        each recipe is written to <dir>/<slug>.json\n" +
			"                  (a trailing slash or an existing directory)\n" +
			"  -O <file>       a single recipe is written to that file\n\n" +
			"--all exports every recipe and requires -O <dir>. Existing files are\n" +
			"never overwritten unless --force is given.",
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			// export always writes raw recipe JSON, bypassing the table/format
			// machinery. table/json both map to raw JSON; yaml/ndjson are rejected
			// rather than silently ignored.
			switch p.Format {
			case output.FormatTable, output.FormatJSON:
			default:
				return usageError(fmt.Sprintf("recipe export does not support --output %s; it writes raw recipe JSON (use table or json)", p.Format))
			}

			if all && len(args) > 0 {
				return usageError("--all exports every recipe; do not also pass slugs")
			}
			if !all && len(args) == 0 {
				return usageError("provide at least one recipe slug, or use --all")
			}

			toDir := out != "" && isDirDestination(out)
			if all && !toDir {
				return usageError("--all requires -O <dir> to write the exported files into")
			}

			// Resolve the slugs to export.
			slugs := args
			if all {
				items, ferr := core.FetchAll(ctx, 0, func(page, pp int) (*core.Page[core.RecipeSummary], error) {
					return c.ListRecipes(ctx, core.RecipeListOptions{ListOptions: core.ListOptions{Page: page, PerPage: pp}})
				})
				if ferr != nil {
					return ferr
				}
				slugs = make([]string, 0, len(items))
				for _, it := range items {
					slugs = append(slugs, it.Slug)
				}
			}

			// stdout: a single recipe, composable and contract-safe.
			if out == "" {
				if len(slugs) != 1 {
					return usageError("writing multiple recipes requires -O <dir>")
				}
				raw, gerr := c.ExportRecipe(ctx, slugs[0])
				if gerr != nil {
					return gerr
				}
				return writeRawJSON(p.Out, raw)
			}

			// -O <file>: a single recipe only.
			if !toDir {
				if len(slugs) != 1 {
					return usageError("-O <file> writes a single recipe; use -O <dir> for multiple recipes")
				}
				if guardErr := guardOverwrite(out, force); guardErr != nil {
					return guardErr
				}
				raw, gerr := c.ExportRecipe(ctx, slugs[0])
				if gerr != nil {
					return gerr
				}
				if werr := writeFileAtomic(out, raw); werr != nil {
					return werr
				}
				p.Info("Exported %s → %s", slugs[0], out)
				return emitExported(p, []string{out}, "")
			}

			// -O <dir>: write <slug>.json for each recipe. As a backup tool this is
			// fail-hard and all-or-nothing — validate filenames, compute every
			// target path, and reject duplicate destinations and pre-existing files
			// BEFORE any write.
			paths := make([]string, len(slugs))
			seen := make(map[string]bool, len(slugs))
			for i, s := range slugs {
				if !safeSlugFilename(s) {
					return usageError(fmt.Sprintf("unsafe recipe slug %q: cannot be used as a filename", s))
				}
				path := filepath.Join(out, s+".json")
				if seen[path] {
					return usageError(fmt.Sprintf("duplicate export destination %s (two recipes map to the same file)", path))
				}
				seen[path] = true
				paths[i] = path
			}
			if mkErr := os.MkdirAll(out, 0o755); mkErr != nil {
				return mkErr
			}
			if !force {
				for _, path := range paths {
					if _, statErr := os.Stat(path); statErr == nil {
						return overwriteError(path)
					}
				}
			}

			// Stage every recipe to a temp file beside its destination first, so a
			// fetch or write failure mid-run aborts before any destination is
			// touched and existing backups are left intact. Only once the whole set
			// is staged do we publish by renaming each temp into place.
			staged := make([]string, 0, len(slugs)) // temp paths, slug-aligned
			cleanup := func() {
				for _, tmp := range staged {
					_ = os.Remove(tmp)
				}
			}
			for _, s := range slugs {
				raw, gerr := c.ExportRecipe(ctx, s)
				if gerr != nil {
					cleanup()
					return gerr
				}
				tmp, werr := writeTempFile(out, raw)
				if werr != nil {
					cleanup()
					return werr
				}
				staged = append(staged, tmp)
				p.Info("Fetched %s", s)
			}
			// Publish. Renames within one directory are atomic and effectively
			// failure-free; if one does fail mid-batch, earlier files are already
			// published (true rollback of a --force overwrite is impossible once the
			// old bytes are gone), so the error reports how far it got.
			written := make([]string, 0, len(staged))
			for i, tmp := range staged {
				if rerr := os.Rename(tmp, paths[i]); rerr != nil {
					// Drop the temps we have not yet published.
					for _, leftover := range staged[i:] {
						_ = os.Remove(leftover)
					}
					return fmt.Errorf("published %d of %d recipes before failing on %s: %w", len(written), len(staged), paths[i], rerr)
				}
				written = append(written, paths[i])
			}
			p.Info("Exported %d recipe(s) to %s", len(written), out)
			return emitExported(p, written, out)
		},
	}
	flags := cmd.Flags()
	flags.StringVarP(&out, "output-file", "O", "", "write to this file or directory (default: stdout for a single recipe)")
	flags.BoolVar(&all, "all", false, "export every recipe (requires -O <dir>)")
	flags.BoolVar(&force, "force", false, "overwrite existing files")
	return cmd
}

// isDirDestination reports whether -O names a directory: a trailing path
// separator, or an already-existing directory.
func isDirDestination(out string) bool {
	if strings.HasSuffix(out, "/") || strings.HasSuffix(out, string(os.PathSeparator)) {
		return true
	}
	info, err := os.Stat(out)
	return err == nil && info.IsDir()
}

// safeSlugFilename reports whether slug can be used as a bare filename. Slugs
// come from user args and the server, and url.PathEscape (used for API paths)
// does not sanitise a filename, so an unguarded <slug>.json is a path-traversal
// write. A slug is safe only when it is its own basename, contains no path
// separator, and has no ".." segment.
func safeSlugFilename(slug string) bool {
	if slug == "" {
		return false
	}
	if strings.ContainsAny(slug, `/\`) {
		return false
	}
	if strings.Contains(slug, "..") {
		return false
	}
	return filepath.Base(slug) == slug
}

// guardOverwrite refuses to clobber an existing file unless force is set.
func guardOverwrite(path string, force bool) error {
	if force {
		return nil
	}
	if _, err := os.Stat(path); err == nil {
		return overwriteError(path)
	}
	return nil
}

func overwriteError(path string) error {
	return newError(ExitConflict, "exists",
		fmt.Sprintf("refusing to overwrite existing file %s", path),
		"re-run with --force to overwrite")
}

// writeTempFile writes data to a freshly created temp file in dir and returns
// its path, leaving it unpublished for the caller to rename into place. The file
// keeps os.CreateTemp's owner-only 0600 mode: exported recipes are lossless and
// can carry private notes/extras/household metadata, so they must not be made
// world- or group-readable (matching the 0600 posture of the config file). On
// any error the temp file is removed.
func writeTempFile(dir string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".mealie-export-*.tmp")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	return name, nil
}

// writeFileAtomic writes data to path via a temp file and rename, so a failure
// mid-write can never leave a torn or partial file — important for a backup
// tool. The file keeps the owner-only 0600 mode (see writeTempFile).
func writeFileAtomic(path string, data []byte) error {
	tmp, err := writeTempFile(filepath.Dir(path), data)
	if err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// writeRawJSON writes the verbatim recipe JSON to w, ensuring a trailing newline
// so terminal output and pipes stay clean.
func writeRawJSON(w io.Writer, raw []byte) error {
	if _, err := w.Write(raw); err != nil {
		return err
	}
	if n := len(raw); n == 0 || raw[n-1] != '\n' {
		if _, err := io.WriteString(w, "\n"); err != nil {
			return err
		}
	}
	return nil
}

// emitExported reports written files: the paths on stdout in human mode, and a
// {"written":[...],"dir":"…"} object in machine mode (dir omitted for a single
// file written outside a directory destination).
func emitExported(p *output.Printer, written []string, dir string) error {
	payload := map[string]any{"written": written}
	if dir != "" {
		payload["dir"] = dir
	}
	return p.Emit(payload, func(w io.Writer) error {
		for _, path := range written {
			fmt.Fprintln(w, path)
		}
		return nil
	})
}

func newRecipeDeleteCmd(f *Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <slug>",
		Aliases: []string{"rm"},
		Short:   "Delete a recipe by slug",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if err := f.confirm(fmt.Sprintf("Delete recipe %q?", args[0]), yes); err != nil {
				return err
			}
			if err := c.DeleteRecipe(ctx, args[0]); err != nil {
				return err
			}
			p.Info("Deleted recipe %q", args[0])
			return p.Emit(map[string]string{"slug": args[0], "status": "deleted"}, func(w io.Writer) error {
				fmt.Fprintf(w, "Deleted %s\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func renderRecipe(w io.Writer, r *core.Recipe) {
	fmt.Fprintln(w, r.Name)
	if r.Description != "" {
		fmt.Fprintf(w, "\n%s\n", r.Description)
	}
	if r.RecipeYield != "" {
		fmt.Fprintf(w, "\nYield: %s\n", r.RecipeYield)
	}
	if r.TotalTime != "" {
		fmt.Fprintf(w, "Total time: %s\n", r.TotalTime)
	}
	if len(r.Tags) > 0 {
		fmt.Fprintf(w, "Tags: %s\n", orgNames(r.Tags))
	}
	if len(r.RecipeIngredient) > 0 {
		fmt.Fprintln(w, "\nIngredients:")
		for _, ing := range r.RecipeIngredient {
			line := firstNonEmpty(ing.Display, ing.OriginalText, ing.Note)
			if ing.Title != "" {
				fmt.Fprintf(w, "  [%s]\n", ing.Title)
			}
			if line != "" {
				fmt.Fprintf(w, "  - %s\n", line)
			}
		}
	}
	if len(r.RecipeInstructions) > 0 {
		fmt.Fprintln(w, "\nInstructions:")
		for i, step := range r.RecipeInstructions {
			if step.Title != "" {
				fmt.Fprintf(w, "  [%s]\n", step.Title)
			}
			fmt.Fprintf(w, "  %d. %s\n", i+1, step.Text)
		}
	}
}
