package core

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError is a non-2xx response from the Mealie API, normalised into a stable
// shape the CLI can map onto exit codes and the JSON error envelope.
type APIError struct {
	StatusCode int
	// Code is a stable machine token derived from the status (e.g. "not_found").
	Code string
	// Message is a single-line human description, extracted from the API body.
	Message string
	// RequestID echoes the server correlation id when present.
	RequestID string
	// Retryable indicates a transient failure (429/502/503/504).
	Retryable bool
	// RetryAfter is the server's suggested wait before retrying, parsed from the
	// Retry-After header (zero when absent).
	RetryAfter time.Duration
	// Fields carries per-field validation errors for 4xx validation responses.
	Fields []FieldError
}

// parseRetryAfter interprets a Retry-After header value, which may be either a
// delay in seconds or an HTTP date. It returns zero when the value is empty or
// unparseable.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// FieldError is a single validation problem reported by the API.
type FieldError struct {
	Location string `json:"location"`
	Message  string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("mealie API error %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("mealie API error %d", e.StatusCode)
}

// TransportError wraps a network/transport-level failure (no HTTP response).
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return fmt.Sprintf("network error: %v", e.Err) }
func (e *TransportError) Unwrap() error { return e.Err }

// AsAPIError extracts an *APIError from err, if present.
func AsAPIError(err error) (*APIError, bool) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr, true
	}
	return nil, false
}

// IsTransport reports whether err is a transport-level failure.
func IsTransport(err error) bool {
	var t *TransportError
	return errors.As(err, &t)
}

func statusIs(err error, code int) bool {
	if apiErr, ok := AsAPIError(err); ok {
		return apiErr.StatusCode == code
	}
	return false
}

// IsNotFound reports a 404 response.
func IsNotFound(err error) bool { return statusIs(err, http.StatusNotFound) }

// IsUnauthorized reports a 401 response (missing/invalid token).
func IsUnauthorized(err error) bool { return statusIs(err, http.StatusUnauthorized) }

// IsForbidden reports a 403 response.
func IsForbidden(err error) bool { return statusIs(err, http.StatusForbidden) }

// IsConflict reports a 409 response.
func IsConflict(err error) bool { return statusIs(err, http.StatusConflict) }

// IsValidation reports a 400 or 422 response.
func IsValidation(err error) bool {
	return statusIs(err, http.StatusBadRequest) || statusIs(err, http.StatusUnprocessableEntity)
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "auth"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusUnprocessableEntity:
		return "validation"
	case http.StatusTooManyRequests:
		return "rate_limited"
	default:
		if status >= 500 {
			return "server_error"
		}
		return "api_error"
	}
}

func joinLoc(loc []any) string {
	parts := make([]string, 0, len(loc))
	for i, v := range loc {
		s := fmt.Sprint(v)
		if i == 0 && (s == "body" || s == "query" || s == "path") {
			continue
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, ".")
}
