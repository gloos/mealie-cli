package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/pkg/core"
)

func newShoppingCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "shopping",
		Aliases: []string{"shop"},
		Short:   "Manage shopping lists",
	}
	cmd.AddCommand(
		newShoppingListCmd(f),
		newShoppingGetCmd(f),
		newShoppingCreateCmd(f),
		newShoppingDeleteCmd(f),
		newShoppingItemCmd(f),
		newShoppingRecipeCmd(f),
	)
	return cmd
}

func newShoppingListCmd(f *Factory) *cobra.Command {
	var (
		page, perPage int
		all           bool
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List shopping lists",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			opts := core.ListOptions{Page: page, PerPage: perPage}
			var res *core.Page[core.ShoppingList]
			if all {
				res, err = fetchAllPage(cmd, perPage, func(page, pp int) (*core.Page[core.ShoppingList], error) {
					o := opts
					o.Page = page
					o.PerPage = pp
					return c.ListShoppingLists(ctx, o)
				})
			} else {
				res, err = c.ListShoppingLists(ctx, opts)
			}
			if err != nil {
				return err
			}
			return emitPage(p, res, func(w io.Writer) error {
				tw := newTable(w, "ID", "NAME", "ITEMS")
				for _, l := range res.Items {
					fmt.Fprintf(tw, "%s\t%s\t%d\n", l.ID, l.Name, len(l.ListItems))
				}
				return tw.Flush()
			})
		},
	}
	flags := cmd.Flags()
	flags.IntVar(&page, "page", 0, "page number (1-based)")
	flags.IntVar(&perPage, "per-page", 0, "results per page")
	flags.BoolVar(&all, "all", false, "fetch every result, paginating client-side in --per-page batches")
	return cmd
}

func newShoppingGetCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "get <list-id>",
		Short: "Show a shopping list and its items",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			list, err := c.GetShoppingList(ctx, args[0])
			if err != nil {
				return err
			}
			return p.Emit(list, func(w io.Writer) error {
				shoppingListDetail(w, list)
				return nil
			})
		},
	}
}

func newShoppingCreateCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "create <name>",
		Short: "Create a new shopping list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			list, err := c.CreateShoppingList(ctx, args[0])
			if err != nil {
				return err
			}
			p.Info("Created shopping list %q (%s)", list.Name, list.ID)
			return p.Emit(map[string]string{"id": list.ID, "name": list.Name}, func(w io.Writer) error {
				fmt.Fprintln(w, list.ID)
				return nil
			})
		},
	}
}

func newShoppingDeleteCmd(f *Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <list-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a shopping list",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if err := f.confirm(fmt.Sprintf("Delete shopping list %q?", args[0]), yes); err != nil {
				return err
			}
			if err := c.DeleteShoppingList(ctx, args[0]); err != nil {
				return err
			}
			p.Info("Deleted shopping list %q", args[0])
			return p.Emit(map[string]string{"id": args[0], "status": "deleted"}, func(w io.Writer) error {
				fmt.Fprintf(w, "Deleted %s\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newShoppingItemCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "item",
		Short: "Manage shopping list items",
	}
	cmd.AddCommand(
		newShoppingItemAddCmd(f),
		newShoppingItemCheckCmd(f, true),
		newShoppingItemCheckCmd(f, false),
		newShoppingItemDeleteCmd(f),
	)
	return cmd
}

func newShoppingItemAddCmd(f *Factory) *cobra.Command {
	var listID string
	var qty float64
	cmd := &cobra.Command{
		Use:   "add <note...>",
		Short: "Add a free-text item to a shopping list",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			in := core.CreateShoppingItem{
				ShoppingListID: listID,
				Note:           strings.Join(args, " "),
			}
			if cmd.Flags().Changed("quantity") {
				in.Quantity = &qty
			}
			item, err := c.AddShoppingItem(ctx, in)
			if err != nil {
				return err
			}
			p.Info("Added item to list %s", listID)
			return p.Emit(item, func(w io.Writer) error {
				fmt.Fprintf(w, "Added %s\n", firstNonEmpty(item.Display, item.Note))
				return nil
			})
		},
	}
	cmd.Flags().StringVar(&listID, "list", "", "shopping list id (required)")
	cmd.Flags().Float64Var(&qty, "quantity", 0, "quantity for the item")
	_ = cmd.MarkFlagRequired("list")
	return cmd
}

func newShoppingItemCheckCmd(f *Factory, checked bool) *cobra.Command {
	use, short, past := "check <item-id>", "Check off a shopping list item", "Checked"
	if !checked {
		use, short, past = "uncheck <item-id>", "Uncheck a shopping list item", "Unchecked"
	}
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			item, err := c.SetItemChecked(ctx, args[0], checked)
			if err != nil {
				return err
			}
			p.Info("%s item %s", past, args[0])
			return p.Emit(item, func(w io.Writer) error {
				box := "[ ]"
				if item.Checked {
					box = "[x]"
				}
				fmt.Fprintf(w, "%s %s\n", box, firstNonEmpty(item.Display, item.Note))
				return nil
			})
		},
	}
}

func newShoppingItemDeleteCmd(f *Factory) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:     "delete <item-id>",
		Aliases: []string{"rm"},
		Short:   "Delete a shopping list item",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if err := f.confirm(fmt.Sprintf("Delete shopping item %q?", args[0]), yes); err != nil {
				return err
			}
			if err := c.DeleteShoppingItem(ctx, args[0]); err != nil {
				return err
			}
			p.Info("Deleted item %q", args[0])
			return p.Emit(map[string]string{"id": args[0], "status": "deleted"}, func(w io.Writer) error {
				fmt.Fprintf(w, "Deleted %s\n", args[0])
				return nil
			})
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	return cmd
}

func newShoppingRecipeCmd(f *Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipe",
		Short: "Manage recipes on a shopping list",
	}
	cmd.AddCommand(
		newShoppingRecipeAddCmd(f),
	)
	return cmd
}

func newShoppingRecipeAddCmd(f *Factory) *cobra.Command {
	var listID, recipeID string
	var scale float64
	cmd := &cobra.Command{
		Use:   "add <recipe-slug>",
		Short: "Add a recipe's ingredients to a shopping list",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, p, err := f.clientPrinter(ctx)
			if err != nil {
				return err
			}
			if scale <= 0 {
				return usageError(fmt.Sprintf("--scale must be positive, got %v", scale))
			}
			// Resolve the slug to a recipe id the same way `mealplan add` does;
			// --recipe-id skips the lookup.
			if recipeID == "" {
				r, gerr := c.GetRecipe(ctx, args[0])
				if gerr != nil {
					return gerr
				}
				recipeID = r.ID
			}
			req := core.AddRecipeToList{RecipeID: recipeID}
			// Only send a scale when the user set one, so an untouched flag leaves
			// the server's default of 1 in place rather than overriding it.
			if cmd.Flags().Changed("scale") {
				req.Scale = scale
			}
			list, err := c.AddRecipesToShoppingList(ctx, listID, []core.AddRecipeToList{req})
			if err != nil {
				return err
			}
			// listItems is the whole list, not just what we added, so the count is
			// reported as a total ("now N items") rather than an added count.
			p.Info("Added recipe to list %s (now %d items)", listID, len(list.ListItems))
			return p.Emit(list, func(w io.Writer) error {
				shoppingListDetail(w, list)
				return nil
			})
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&listID, "list", "", "shopping list id (required)")
	flags.Float64Var(&scale, "scale", 1, "recipe scale factor (servings multiplier)")
	flags.StringVar(&recipeID, "recipe-id", "", "recipe UUID (skips the slug lookup)")
	_ = cmd.MarkFlagRequired("list")
	return cmd
}

func shoppingListDetail(w io.Writer, list *core.ShoppingList) {
	fmt.Fprintf(w, "%s (%s)\n", list.Name, list.ID)
	if len(list.ListItems) == 0 {
		fmt.Fprintln(w, "(empty)")
		return
	}
	tw := newTable(w, "", "ID", "QTY", "ITEM")
	for _, it := range list.ListItems {
		box := "[ ]"
		if it.Checked {
			box = "[x]"
		}
		qty := ""
		if it.Quantity != nil {
			qty = strconv.FormatFloat(*it.Quantity, 'g', -1, 64)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", box, it.ID, dash(qty), dash(firstNonEmpty(it.Display, it.Note)))
	}
	tw.Flush()
}
