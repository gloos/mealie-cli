package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/pkg/core"
)

func newMealplanCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "mealplan",
		Aliases: []string{"mealplans", "plan"},
		Short:   "Manage meal plans",
	}
	cmd.AddCommand(
		newMealplanListCmd(f),
		newMealplanTodayCmd(f),
		newMealplanAddCmd(f),
		newMealplanDeleteCmd(f),
	)
	return cmd
}

func mealplanTable(w io.Writer, items []core.MealPlan) error {
	tw := newTable(w, "ID", "DATE", "TYPE", "ENTRY")
	for _, m := range items {
		entry := m.Title
		if entry == "" && m.Recipe != nil {
			entry = m.Recipe.Name
		}
		if entry == "" {
			entry = m.Text
		}
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\n", m.ID, m.Date, m.EntryType, dash(entry))
	}
	return tw.Flush()
}

func newMealplanListCmd(f *Factory) *cobra.Command {
	var (
		start, end    string
		page, perPage int
		all           bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List meal plan entries, optionally within a date range",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			opts := core.ListOptions{Page: page, PerPage: perPage}
			var res *core.Page[core.MealPlan]
			if all {
				res, err = fetchAllPage(cmd, perPage, func(page, pp int) (*core.Page[core.MealPlan], error) {
					o := opts
					o.Page = page
					o.PerPage = pp
					return c.ListMealPlans(ctx, o, start, end)
				})
			} else {
				res, err = c.ListMealPlans(ctx, opts, start, end)
			}
			if err != nil {
				return err
			}
			return emitPage(p, res, func(w io.Writer) error {
				return mealplanTable(w, res.Items)
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&start, "start", "", "start date (YYYY-MM-DD)")
	flags.StringVar(&end, "end", "", "end date (YYYY-MM-DD)")
	flags.IntVar(&page, "page", 0, "page number (1-based)")
	flags.IntVar(&perPage, "per-page", 0, "results per page")
	flags.BoolVar(&all, "all", false, "fetch every result, paginating client-side in --per-page batches")
	return cmd
}

func newMealplanTodayCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "today",
		Short: "Show today's meal plan entries",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			entries, err := c.TodayMealPlans(ctx)
			if err != nil {
				return err
			}
			return p.Emit(entries, func(w io.Writer) error {
				return mealplanTable(w, entries)
			})
		},
	}
}

func newMealplanAddCmd(f *Factory) *cobra.Command {
	var date, entryType, recipeSlug, recipeID, title, text string
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a meal plan entry",
		Long: "Add a meal plan entry. Plan an existing recipe with --recipe <slug>\n" +
			"(or --recipe-id <uuid>), or add a free-text entry with --title/--text.\n" +
			"The date defaults to today.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if !validEntryType(entryType) {
				return usageError(fmt.Sprintf("invalid --type %q (want one of: %s)", entryType, strings.Join(core.EntryTypes, ", ")))
			}
			if date == "" {
				date = time.Now().Format("2006-01-02")
			}
			if recipeID == "" && recipeSlug != "" {
				r, gerr := c.GetRecipe(ctx, recipeSlug)
				if gerr != nil {
					return gerr
				}
				recipeID = r.ID
			}
			if recipeID == "" && title == "" && text == "" {
				return usageError("provide --recipe/--recipe-id or --title/--text")
			}
			entry, err := c.CreateMealPlan(ctx, core.CreateMealPlan{
				Date:      date,
				EntryType: entryType,
				Title:     title,
				Text:      text,
				RecipeID:  recipeID,
			})
			if err != nil {
				return err
			}
			p.Info("Added %s entry for %s (id %d)", entry.EntryType, entry.Date, entry.ID)
			return p.Emit(entry, func(w io.Writer) error {
				return mealplanTable(w, []core.MealPlan{*entry})
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&date, "date", "", "date for the entry (YYYY-MM-DD, default today)")
	flags.StringVar(&entryType, "type", core.EntryDinner, "entry type: "+strings.Join(core.EntryTypes, "|"))
	flags.StringVar(&recipeSlug, "recipe", "", "slug of an existing recipe to plan")
	flags.StringVar(&recipeID, "recipe-id", "", "UUID of an existing recipe to plan")
	flags.StringVar(&title, "title", "", "title for a free-text entry")
	flags.StringVar(&text, "text", "", "note text for a free-text entry")
	return cmd
}

func newMealplanDeleteCmd(f *Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <id>",
		Aliases: []string{"rm"},
		Short:   "Delete a meal plan entry by id",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			id, perr := strconv.Atoi(args[0])
			if perr != nil {
				return usageError(fmt.Sprintf("invalid entry id %q (must be a number)", args[0]))
			}
			if err := f.confirm(fmt.Sprintf("Delete meal plan entry %d?", id), yes); err != nil {
				return err
			}
			if err := c.DeleteMealPlan(ctx, id); err != nil {
				return err
			}
			p.Info("Deleted meal plan entry %d", id)
			return p.Emit(map[string]any{"id": id, "status": "deleted"}, func(w io.Writer) error {
				fmt.Fprintf(w, "Deleted entry %d\n", id)
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func validEntryType(t string) bool {
	for _, e := range core.EntryTypes {
		if e == t {
			return true
		}
	}
	return false
}
