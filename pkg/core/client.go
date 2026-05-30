package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultUserAgent   = "mealie-cli"
	defaultTimeout     = 30 * time.Second
	defaultRetries     = 2
	defaultBackoffBase = 200 * time.Millisecond
	maxBackoff         = 5 * time.Second
)

// Client is a Mealie API client. It is safe for concurrent use once
// constructed; all configuration happens via options in New.
type Client struct {
	baseURL     string
	token       string
	httpClient  *http.Client
	timeout     time.Duration
	userAgent   string
	maxRetries  int
	backoffBase time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient supplies a custom *http.Client (e.g. for proxies or test
// transports). When provided, the client is used as-is and is never mutated;
// WithTimeout is then ignored (set the timeout on your own client instead).
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithUserAgent sets the User-Agent header, e.g. "mealie-cli/1.2.0".
func WithUserAgent(ua string) Option {
	return func(c *Client) {
		if ua != "" {
			c.userAgent = ua
		}
	}
}

// WithTimeout sets the per-request timeout applied to the client New builds. It
// has no effect when a custom client is supplied via WithHTTPClient.
func WithTimeout(d time.Duration) Option {
	return func(c *Client) {
		if d > 0 {
			c.timeout = d
		}
	}
}

// WithMaxRetries sets how many times transient failures are retried for
// idempotent methods (GET/HEAD/PUT/DELETE). POST is never auto-retried.
func WithMaxRetries(n int) Option {
	return func(c *Client) {
		if n >= 0 {
			c.maxRetries = n
		}
	}
}

// New constructs a Client for the given server base URL (e.g.
// https://mealie.example.com, without the /api suffix) and API token. The token
// may be empty for unauthenticated calls such as Login or the public About.
func New(baseURL, token string, opts ...Option) (*Client, error) {
	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	c := &Client{
		baseURL:     strings.TrimRight(baseURL, "/"),
		token:       token,
		userAgent:   defaultUserAgent,
		maxRetries:  defaultRetries,
		timeout:     defaultTimeout,
		backoffBase: defaultBackoffBase,
	}
	for _, o := range opts {
		o(c)
	}
	// Build our own client from the timeout unless the caller supplied one. A
	// caller-supplied client is respected as-is and never mutated.
	if c.httpClient == nil {
		c.httpClient = &http.Client{Timeout: c.timeout}
	}
	return c, nil
}

// BaseURL returns the configured server root.
func (c *Client) BaseURL() string { return c.baseURL }

// do issues a JSON request authenticated with the client token and decodes a
// JSON response into out (which may be nil).
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var raw []byte
	contentType := ""
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		raw = b
		contentType = "application/json"
	}
	return c.request(ctx, c.token, method, path, query, contentType, raw, out)
}

// request is the transport core shared by do and the auth flow. token, query,
// contentType and bodyBytes may be empty.
func (c *Client) request(ctx context.Context, token, method, path string, query url.Values, contentType string, bodyBytes []byte, out any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}

	idempotent := isIdempotent(method)
	attempts := 1
	if idempotent {
		attempts += c.maxRetries
	}

	var lastErr error
	var wait time.Duration // delay before the next attempt (server hint or backoff)
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			if wait <= 0 {
				wait = c.backoff(attempt)
			}
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return ctx.Err()
				case <-timer.C:
				}
			} else if err := ctx.Err(); err != nil {
				return err
			}
			wait = 0
		}

		var reader io.Reader
		if bodyBytes != nil {
			reader = bytes.NewReader(bodyBytes)
		}
		req, err := http.NewRequestWithContext(ctx, method, u, reader)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", c.userAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = &TransportError{Err: err}
			if idempotent && attempt < attempts-1 {
				continue
			}
			return lastErr
		}

		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			lastErr = &TransportError{Err: readErr}
			if idempotent && attempt < attempts-1 {
				continue
			}
			return lastErr
		}

		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			if out != nil && len(bytes.TrimSpace(respBody)) > 0 {
				if err := json.Unmarshal(respBody, out); err != nil {
					return fmt.Errorf("decode response from %s %s: %w", method, path, err)
				}
			}
			return nil
		}

		apiErr := parseAPIError(resp.StatusCode, resp.Header, respBody)
		if apiErr.Retryable && idempotent && attempt < attempts-1 {
			lastErr = apiErr
			wait = apiErr.RetryAfter // honour server hint; 0 falls back to backoff
			continue
		}
		return apiErr
	}
	return lastErr
}

func isIdempotent(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPut, http.MethodDelete, http.MethodOptions:
		return true
	default:
		return false
	}
}

// backoff returns an exponential delay with full jitter, capped at maxBackoff.
// A non-positive backoffBase disables waiting (used in tests).
func (c *Client) backoff(attempt int) time.Duration {
	if c.backoffBase <= 0 {
		return 0
	}
	d := c.backoffBase << (attempt - 1)
	if d > maxBackoff {
		d = maxBackoff
	}
	return time.Duration(rand.Int64N(int64(d)) + int64(c.backoffBase))
}

func parseAPIError(status int, header http.Header, body []byte) *APIError {
	e := &APIError{
		StatusCode: status,
		Code:       codeForStatus(status),
		RequestID:  header.Get("X-Request-Id"),
		Retryable:  status == http.StatusTooManyRequests || status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout,
	}
	if ra := parseRetryAfter(header.Get("Retry-After")); ra > 0 {
		e.RetryAfter = ra
	}

	var probe struct {
		Detail json.RawMessage `json:"detail"`
	}
	if len(body) > 0 && json.Unmarshal(body, &probe) == nil && len(probe.Detail) > 0 {
		var msg string
		if json.Unmarshal(probe.Detail, &msg) == nil {
			e.Message = msg
		} else {
			var items []struct {
				Loc []any  `json:"loc"`
				Msg string `json:"msg"`
			}
			if json.Unmarshal(probe.Detail, &items) == nil {
				for _, it := range items {
					e.Fields = append(e.Fields, FieldError{Location: joinLoc(it.Loc), Message: it.Msg})
				}
				if len(e.Fields) > 0 {
					if loc := e.Fields[0].Location; loc != "" {
						e.Message = loc + ": " + e.Fields[0].Message
					} else {
						e.Message = e.Fields[0].Message
					}
				}
			}
		}
	}
	if e.Message == "" {
		if txt := http.StatusText(status); txt != "" {
			e.Message = txt
		} else {
			e.Message = "request failed"
		}
	}
	return e
}
