package output

import (
	"encoding/json"
	"fmt"
)

// ErrorPayload is the stable, machine-readable error contract. In any machine
// output format it is emitted to stderr wrapped in an {"error": {...}} envelope,
// so agents can branch on `code`, `http_status` and `retryable` without parsing
// prose. Treat the field set and `code` vocabulary as a versioned API.
type ErrorPayload struct {
	// Code is a stable, lowercase machine token, e.g. "not_found", "auth", "validation".
	Code string `json:"code"`
	// Message is a human-readable, single-line description.
	Message string `json:"message"`
	// HTTPStatus is the upstream Mealie status code, when the error came from the API.
	HTTPStatus int `json:"http_status,omitempty"`
	// Retryable indicates the operation may succeed if retried (transient).
	Retryable bool `json:"retryable"`
	// RequestID echoes a server/request correlation id when available.
	RequestID string `json:"request_id,omitempty"`
	// Details carries structured, error-specific context (e.g. validation fields).
	Details map[string]any `json:"details,omitempty"`
	// Hint is an actionable next step for humans (e.g. "run `mealie auth login`").
	Hint string `json:"hint,omitempty"`
}

type errorEnvelope struct {
	Error ErrorPayload `json:"error"`
}

// EmitError writes the error to stderr. Machine formats emit the JSON envelope;
// table mode emits a concise human message plus an optional hint. Errors always
// go to stderr so stdout stays a clean data channel.
func (p *Printer) EmitError(pl ErrorPayload) {
	if p.Format.Machine() {
		enc := json.NewEncoder(p.Err)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		_ = enc.Encode(errorEnvelope{Error: pl})
		return
	}
	fmt.Fprintf(p.Err, "Error: %s\n", pl.Message)
	if pl.Hint != "" {
		fmt.Fprintf(p.Err, "Hint:  %s\n", pl.Hint)
	}
}
