package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/internal/buildinfo"
	"github.com/gloos/mealie-cli/pkg/output"
)

const rootLong = `Mealie CLI — a fast, scriptable, agent-friendly client for Mealie.

Manage recipes, meal plans and shopping lists from the terminal or from scripts
and agents. Every command supports machine-readable output (--output json|ndjson)
with data on stdout, diagnostics on stderr, and stable exit codes.

Getting started:
  mealie auth login --url https://mealie.example.com
  mealie doctor
  mealie recipe list --search curry --output json

Configuration precedence: flags > environment (MEALIE_URL, MEALIE_TOKEN,
MEALIE_PROFILE, MEALIE_OUTPUT, MEALIE_CONFIG) > profile file > defaults.`

// NewRootCommand builds the root command and wires every subcommand to the
// supplied factory.
func NewRootCommand(f *Factory) *cobra.Command {
	root := &cobra.Command{
		Use:           "mealie",
		Short:         "Manage Mealie recipes, meal plans and shopping lists",
		Long:          rootLong,
		Version:       buildinfo.Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(f.Out)
	root.SetErr(f.Err)
	root.SetIn(f.In)

	pf := root.PersistentFlags()
	pf.StringVarP(&f.opts.profile, "profile", "p", "", "configuration profile (env MEALIE_PROFILE)")
	pf.StringVar(&f.opts.url, "url", "", "Mealie server base URL (env MEALIE_URL)")
	pf.StringVar(&f.opts.token, "token", "", "API token (env MEALIE_TOKEN)")
	pf.StringVarP(&f.opts.output, "output", "o", "", "output format: table|json|ndjson|yaml (env MEALIE_OUTPUT)")
	pf.BoolVarP(&f.opts.quiet, "quiet", "q", false, "suppress incidental messages on stderr")
	pf.BoolVar(&f.opts.noInput, "no-input", false, "never prompt; fail instead (for scripts and agents)")
	pf.BoolVar(&f.opts.noColor, "no-color", false, "disable colour output")
	pf.StringVar(&f.opts.configPath, "config", "", "config file path (env MEALIE_CONFIG)")
	pf.DurationVar(&f.opts.timeout, "timeout", 30*time.Second, "HTTP request timeout")

	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &cliError{code: ExitUsage, payload: output.ErrorPayload{Code: "usage", Message: err.Error()}, err: err}
	})

	root.AddCommand(
		newVersionCmd(f),
		newDoctorCmd(f),
		newAuthCmd(f),
		newConfigCmd(f),
		newRecipeCmd(f),
		newMealplanCmd(f),
		newShoppingCmd(f),
		newSchemaCmd(f),
	)
	return root
}

// run builds the root command, executes it with the supplied context and args,
// and returns the process exit code — classifying any error onto the exit-code
// contract and writing the stable error envelope to stderr. It is the testable
// core of Main: everything that defines the agent contract (run → classify →
// EmitError → exit code) lives here, driven entirely by the injected Factory, so
// a test can exercise the real path rather than a copy that could drift.
func run(ctx context.Context, f *Factory, args []string) int {
	root := NewRootCommand(f)
	root.SetArgs(args)
	if err := root.ExecuteContext(ctx); err != nil {
		code, payload := classify(err)
		if p, perr := f.Printer(); perr == nil {
			p.EmitError(payload)
		} else {
			fmt.Fprintln(f.Err, "Error:", payload.Message)
		}
		return code
	}
	return 0
}

// Main is the program entry point. It returns the process exit code. It wires
// the real process streams and the signal-derived context, then defers all
// contract behaviour to run; keeping Main this thin means the subprocess smoke
// test (which execs the real binary) covers every branch it adds over run.
func Main() int {
	f := &Factory{
		opts:   &globalOptions{},
		getenv: os.Getenv,
		In:     os.Stdin,
		Out:    os.Stdout,
		Err:    os.Stderr,
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, f, os.Args[1:])
}
