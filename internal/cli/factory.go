package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/gloos/mealie-cli/internal/config"
	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

// globalOptions holds the values bound to the root command's persistent flags.
type globalOptions struct {
	profile    string
	url        string
	token      string
	output     string
	quiet      bool
	noInput    bool
	noColor    bool
	configPath string
	timeout    time.Duration
}

// Factory builds the per-invocation dependencies (config, client, printer) from
// the global flags and environment. It is the single seam the commands depend
// on, which keeps them thin and testable.
type Factory struct {
	opts   *globalOptions
	getenv func(string) string
	In     io.Reader
	Out    io.Writer
	Err    io.Writer
}

func (f *Factory) configFilePath() (string, error) {
	if f.opts.configPath != "" {
		return f.opts.configPath, nil
	}
	return config.DefaultPath(f.getenv)
}

// resolved loads the config file and applies flag/env precedence.
func (f *Factory) resolved() (config.Resolved, *config.Config, string, error) {
	path, err := f.configFilePath()
	if err != nil {
		return config.Resolved{}, nil, "", err
	}
	cfg, err := config.Load(path)
	if err != nil {
		return config.Resolved{}, nil, path, err
	}
	res, err := cfg.Resolve(config.Overrides{
		Profile: f.opts.profile,
		BaseURL: f.opts.url,
		Token:   f.opts.token,
	}, f.getenv)
	return res, cfg, path, err
}

// Printer constructs the output sink from the resolved format and TTY state.
func (f *Factory) Printer() (*output.Printer, error) {
	format, err := output.ParseFormat(firstNonEmpty(f.opts.output, f.getenv("MEALIE_OUTPUT")))
	if err != nil {
		return nil, usageError(err.Error())
	}
	color := !f.opts.noColor && f.getenv("NO_COLOR") == "" && isTerminal(f.Out)
	return &output.Printer{
		Out:    f.Out,
		Err:    f.Err,
		Format: format,
		Quiet:  f.opts.quiet,
		Color:  color,
	}, nil
}

// Client builds an authenticated Mealie client, failing with a clear config
// error if the server URL or token is missing.
func (f *Factory) Client(_ context.Context) (*core.Client, error) {
	res, _, _, err := f.resolved()
	if err != nil {
		return nil, newError(ExitConfig, "config", err.Error(), "")
	}
	if res.BaseURL == "" {
		return nil, newError(ExitConfig, "config", "no Mealie server configured",
			"run `mealie auth login` or set --url / MEALIE_URL")
	}
	if res.Token == "" {
		return nil, newError(ExitConfig, "auth", "no API token configured",
			"run `mealie auth login` or set --token / MEALIE_TOKEN")
	}
	f.warnInsecureTransport(res.BaseURL, res.Token)
	return core.New(res.BaseURL, res.Token,
		core.WithUserAgent(userAgent()),
		core.WithTimeout(f.opts.timeout),
	)
}

// warnInsecureTransport warns on stderr when a token would be sent over a
// plaintext http connection to a non-loopback host, where it could be captured
// in transit. http to localhost is common for self-hosted Mealie and is not
// flagged. The warning is intentionally not silenced by --quiet.
func (f *Factory) warnInsecureTransport(baseURL, token string) {
	if token == "" {
		return
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" {
		return
	}
	host := u.Hostname()
	if host == "localhost" {
		return
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return
	}
	fmt.Fprintf(f.Err, "warning: sending your API token over an unencrypted http connection to %s; "+
		"anyone on the network path can read it. Use https to protect it in transit.\n", host)
}

// clientPrinter is the common setup for commands that talk to the API.
func (f *Factory) clientPrinter(ctx context.Context) (*core.Client, *output.Printer, error) {
	p, err := f.Printer()
	if err != nil {
		return nil, nil, err
	}
	c, err := f.Client(ctx)
	if err != nil {
		return nil, nil, err
	}
	return c, p, nil
}
