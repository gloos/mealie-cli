package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
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

	// httpClient, when non-nil, is injected into every core client this factory
	// builds (see coreOptions). Production leaves it nil so core builds its own
	// client from the timeout; tests set it to route requests at a test server,
	// which lets the credential-transport guard be exercised end-to-end against a
	// non-loopback URL.
	httpClient *http.Client
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
	return core.New(res.BaseURL, res.Token, f.coreOptions()...)
}

// coreOptions is the option set shared by every core client this factory builds
// — the API commands, `auth login`, and `doctor`. Centralising it keeps all
// credential-bearing requests on identical transport configuration (user agent,
// timeout, and the injected HTTP client used by tests) so no path can silently
// drift away from the others.
func (f *Factory) coreOptions() []core.Option {
	opts := []core.Option{
		core.WithUserAgent(userAgent()),
		core.WithTimeout(f.opts.timeout),
	}
	if f.httpClient != nil {
		opts = append(opts, core.WithHTTPClient(f.httpClient))
	}
	return opts
}

// insecureTransportHost returns the host of baseURL when it is a plaintext http
// connection to a non-loopback host — where anything sent over it can be read in
// transit — or "" when the transport is safe. https is always safe; http to a
// loopback host (localhost, 127.0.0.1, ::1) is common for a self-hosted Mealie
// and is not flagged.
func insecureTransportHost(baseURL string) string {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" {
		return ""
	}
	host := u.Hostname()
	if host == "localhost" {
		return ""
	}
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		return ""
	}
	return host
}

// warnInsecureTransport warns on stderr when an API token would be sent over an
// insecure plaintext connection. It gates on a token being present, so it stays
// silent for public requests (e.g. doctor's About probe). The warning is
// intentionally not silenced by --quiet.
func (f *Factory) warnInsecureTransport(baseURL, token string) {
	if token == "" {
		return
	}
	if host := insecureTransportHost(baseURL); host != "" {
		fmt.Fprintf(f.Err, "warning: sending your API token over an unencrypted http connection to %s; "+
			"anyone on the network path can read it. Use https to protect it in transit.\n", host)
	}
}

// warnInsecureCredentials warns on stderr before `auth login` sends credentials
// over an insecure plaintext connection. Unlike warnInsecureTransport it does
// not gate on a token, because the password-login flow sends the username and
// password — and then the freshly minted token — before any token is stored, and
// the token-login flow sends the supplied token; one call after URL
// normalisation covers every credential-bearing request the command makes.
func (f *Factory) warnInsecureCredentials(baseURL string) {
	if host := insecureTransportHost(baseURL); host != "" {
		fmt.Fprintf(f.Err, "warning: sending your login credentials over an unencrypted http connection to %s; "+
			"anyone on the network path can read them. Use https to protect them in transit.\n", host)
	}
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
