package cli

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/gloos/mealie-cli/internal/buildinfo"
)

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func userAgent() string {
	return "mealie-cli/" + buildinfo.Version
}

// isTerminal reports whether w is an interactive terminal.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// isReaderTerminal reports whether r is an interactive terminal.
func isReaderTerminal(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// confirm gates a destructive action. With assumeYes it returns nil immediately.
// In non-interactive contexts it fails rather than hanging or consuming piped
// stdin: we require BOTH the prompt stream (stderr) and the input stream (stdin)
// to be a TTY, otherwise the caller must pass --yes.
func (f *Factory) confirm(prompt string, assumeYes bool) error {
	if assumeYes {
		return nil
	}
	if f.opts.noInput || !isTerminal(f.Err) || !isReaderTerminal(f.In) {
		return newError(ExitUsage, "confirmation_required",
			"refusing to perform a destructive action without confirmation",
			"re-run with --yes to confirm")
	}
	fmt.Fprintf(f.Err, "%s [y/N]: ", prompt)
	line, _ := bufio.NewReader(f.In).ReadString('\n')
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return nil
	default:
		return newError(ExitError, "cancelled", "cancelled", "")
	}
}

// promptLine reads a single line of input, displaying prompt on stderr.
func (f *Factory) promptLine(prompt string) (string, error) {
	if f.opts.noInput {
		return "", usageError("input required but --no-input is set")
	}
	fmt.Fprint(f.Err, prompt)
	line, err := bufio.NewReader(f.In).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

// promptPassword reads a secret without echoing when stdin is a TTY.
func (f *Factory) promptPassword(prompt string) (string, error) {
	if f.opts.noInput {
		return "", usageError("password required but --no-input is set")
	}
	fmt.Fprint(f.Err, prompt)
	if in, ok := f.In.(*os.File); ok && term.IsTerminal(int(in.Fd())) {
		b, err := term.ReadPassword(int(in.Fd()))
		fmt.Fprintln(f.Err)
		return string(b), err
	}
	line, _ := bufio.NewReader(f.In).ReadString('\n')
	return strings.TrimRight(line, "\r\n"), nil
}
