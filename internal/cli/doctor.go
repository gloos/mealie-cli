package cli

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/gloos/mealie-cli/internal/buildinfo"
	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

type checkResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn | fail
	Detail string `json:"detail"`
}

type doctorReport struct {
	OK     bool          `json:"ok"`
	Checks []checkResult `json:"checks"`
}

func newDoctorCmd(f *Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose connectivity, authentication and server compatibility",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			p, err := f.Printer()
			if err != nil {
				return err
			}

			res, _, path, rerr := f.resolved()
			var checks []checkResult

			if rerr != nil {
				checks = append(checks, checkResult{"config", "fail", rerr.Error()})
				return finishDoctor(p, checks, rerr)
			}
			checks = append(checks, checkResult{"config", "ok", "profile " + strconv.Quote(res.Profile) + " (" + path + ")"})

			if res.BaseURL == "" {
				checks = append(checks, checkResult{"server-url", "fail", "no server URL configured"})
				return finishDoctor(p, checks, newError(ExitConfig, "config",
					"no Mealie server configured", "run `mealie auth login` or set --url / MEALIE_URL"))
			}
			checks = append(checks, checkResult{"server-url", "ok", res.BaseURL})

			// The connectivity and version probe only hits the public About
			// endpoint, so build a tokenless client for it: core attaches the token
			// to every request, and doctor must not put the token on the wire before
			// the insecure-transport warning — nor at all on a public probe. The
			// authenticated client is built below, only for the Whoami check.
			client, cerr := core.New(res.BaseURL, "", f.coreOptions()...)
			if cerr != nil {
				checks = append(checks, checkResult{"client", "fail", cerr.Error()})
				return finishDoctor(p, checks, cerr)
			}

			about, aerr := client.About(ctx)
			if aerr != nil {
				checks = append(checks, checkResult{"connectivity", "fail", aerr.Error()})
				return finishDoctor(p, checks, aerr)
			}
			checks = append(checks, checkResult{"connectivity", "ok", "reached Mealie " + about.Version})

			switch {
			case !isNumericVersion(about.Version):
				checks = append(checks, checkResult{"version", "ok", "server version " + about.Version + " is non-numeric (e.g. nightly); assuming compatible"})
			case versionAtLeast(about.Version, buildinfo.MinMealieVersion):
				checks = append(checks, checkResult{"version", "ok", "server " + about.Version + " >= min " + buildinfo.MinMealieVersion})
			default:
				checks = append(checks, checkResult{"version", "warn", "server " + about.Version + " is below the supported minimum " + buildinfo.MinMealieVersion})
			}

			if res.Token == "" {
				checks = append(checks, checkResult{"auth", "warn", "no API token configured; only public endpoints are available"})
				return finishDoctor(p, checks, nil)
			}
			// Warn before the token reaches the wire for the first time, then build
			// the authenticated client used solely for the Whoami check.
			f.warnInsecureTransport(res.BaseURL, res.Token)
			authClient, cerr := core.New(res.BaseURL, res.Token, f.coreOptions()...)
			if cerr != nil {
				checks = append(checks, checkResult{"client", "fail", cerr.Error()})
				return finishDoctor(p, checks, cerr)
			}
			user, uerr := authClient.Whoami(ctx)
			if uerr != nil {
				checks = append(checks, checkResult{"auth", "fail", uerr.Error()})
				return finishDoctor(p, checks, uerr)
			}
			checks = append(checks, checkResult{"auth", "ok", "authenticated as " + user.Username})

			return finishDoctor(p, checks, nil)
		},
	}
}

// finishDoctor emits the report (data on stdout) and, when a check failed,
// returns an error whose exit code reflects the underlying failure class — a
// network failure exits 7, an auth failure exits 3 — rather than collapsing
// everything to one code. The report is always emitted before returning.
func finishDoctor(p *output.Printer, checks []checkResult, fatal error) error {
	ok := true
	for _, c := range checks {
		if c.Status == "fail" {
			ok = false
		}
	}
	report := doctorReport{OK: ok, Checks: checks}
	if err := p.Emit(report, func(w io.Writer) error {
		for _, c := range checks {
			fmt.Fprintf(w, "%s  %-14s %s\n", statusGlyph(c.Status), c.Name, c.Detail)
		}
		return nil
	}); err != nil {
		return err
	}
	if ok {
		return nil
	}
	if fatal != nil {
		code, _ := classify(fatal)
		return &cliError{code: code, payload: output.ErrorPayload{Code: "doctor", Message: "one or more checks failed"}}
	}
	return newError(ExitError, "doctor", "one or more checks failed", "")
}

func statusGlyph(status string) string {
	switch status {
	case "ok":
		return "✔"
	case "warn":
		return "!"
	default:
		return "✗"
	}
}

// isNumericVersion reports whether s looks like a numeric version (starts with a
// digit, optionally after a leading "v"). Non-numeric tags such as "nightly"
// cannot be ordered and are treated as compatible by the caller.
func isNumericVersion(s string) bool {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if s == "" {
		return false
	}
	return s[0] >= '0' && s[0] <= '9'
}

func versionAtLeast(have, min string) bool {
	h := parseVersion(have)
	m := parseVersion(min)
	for i := 0; i < 3; i++ {
		if h[i] != m[i] {
			return h[i] > m[i]
		}
	}
	return true
}

func parseVersion(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	parts := strings.SplitN(s, ".", 4)
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		digits := strings.Builder{}
		for _, r := range parts[i] {
			if r >= '0' && r <= '9' {
				digits.WriteRune(r)
			} else {
				break
			}
		}
		out[i], _ = strconv.Atoi(digits.String())
	}
	return out
}
