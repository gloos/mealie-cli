package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/pkg/core"
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
			if all {
				opts.PerPage = -1
			}
			res, err := c.ListRecipes(ctx, opts)
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
	flags.BoolVar(&all, "all", false, "fetch all results (no pagination)")
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
