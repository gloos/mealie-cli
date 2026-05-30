package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type flagSchema struct {
	Name      string `json:"name"`
	Shorthand string `json:"shorthand,omitempty"`
	Usage     string `json:"usage"`
	Type      string `json:"type"`
	Default   string `json:"default,omitempty"`
}

type cmdSchema struct {
	Name        string       `json:"name"`
	Path        string       `json:"path"`
	Short       string       `json:"short,omitempty"`
	Aliases     []string     `json:"aliases,omitempty"`
	GlobalFlags []flagSchema `json:"global_flags,omitempty"`
	Flags       []flagSchema `json:"flags,omitempty"`
	Commands    []cmdSchema  `json:"commands,omitempty"`
}

func newSchemaCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the command tree as JSON for programmatic discovery",
		Long: "Emit the full command tree — every command, alias and flag — as structured\n" +
			"data, so an agent can discover the CLI's capabilities without scraping help text.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := f.Printer()
			if err != nil {
				return err
			}
			root := cmd.Root()
			schema := describeCommand(root)
			schema.GlobalFlags = collectFlags(root.PersistentFlags())
			return p.Emit(schema, func(w io.Writer) error {
				printSchemaTree(w, schema, 0)
				return nil
			})
		},
	}
}

func describeCommand(cmd *cobra.Command) cmdSchema {
	cs := cmdSchema{
		Name:    cmd.Name(),
		Path:    cmd.CommandPath(),
		Short:   cmd.Short,
		Aliases: cmd.Aliases,
		Flags:   collectFlags(cmd.LocalNonPersistentFlags()),
	}
	for _, sub := range cmd.Commands() {
		if sub.Hidden || sub.Name() == "help" {
			continue
		}
		cs.Commands = append(cs.Commands, describeCommand(sub))
	}
	return cs
}

func collectFlags(set *pflag.FlagSet) []flagSchema {
	var flags []flagSchema
	set.VisitAll(func(fl *pflag.Flag) {
		if fl.Hidden {
			return
		}
		flags = append(flags, flagSchema{
			Name:      fl.Name,
			Shorthand: fl.Shorthand,
			Usage:     fl.Usage,
			Type:      fl.Value.Type(),
			Default:   fl.DefValue,
		})
	})
	return flags
}

func printSchemaTree(w io.Writer, cs cmdSchema, depth int) {
	indent := strings.Repeat("  ", depth)
	fmt.Fprintf(w, "%s%s\t%s\n", indent, cs.Path, cs.Short)
	for _, sub := range cs.Commands {
		printSchemaTree(w, sub, depth+1)
	}
}
