package cli

import (
	"errors"

	"github.com/gloos/mealie-cli/pkg/core"
	"github.com/gloos/mealie-cli/pkg/output"
)

// cliError carries an exit code and a machine-readable payload through the cobra
// call stack to the top-level handler in Main.
type cliError struct {
	code    int
	payload output.ErrorPayload
	err     error
}

func (e *cliError) Error() string { return e.payload.Message }
func (e *cliError) Unwrap() error { return e.err }

func newError(code int, machineCode, message, hint string) *cliError {
	return &cliError{code: code, payload: output.ErrorPayload{Code: machineCode, Message: message, Hint: hint}}
}

func usageError(message string) *cliError {
	return newError(ExitUsage, "usage", message, "")
}

// classify converts any error returned from command execution into an exit code
// and a stable error payload.
func classify(err error) (int, output.ErrorPayload) {
	var ce *cliError
	if errors.As(err, &ce) {
		return ce.code, ce.payload
	}

	if apiErr, ok := core.AsAPIError(err); ok {
		payload := output.ErrorPayload{
			Code:       apiErr.Code,
			Message:    apiErr.Message,
			HTTPStatus: apiErr.StatusCode,
			Retryable:  apiErr.Retryable,
			RequestID:  apiErr.RequestID,
		}
		if len(apiErr.Fields) > 0 {
			fields := make([]map[string]string, 0, len(apiErr.Fields))
			for _, fe := range apiErr.Fields {
				fields = append(fields, map[string]string{"location": fe.Location, "message": fe.Message})
			}
			payload.Details = map[string]any{"fields": fields}
		}
		code := ExitError
		switch {
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			code = ExitConfig
			payload.Hint = "check your token or run `mealie auth login`"
		case apiErr.StatusCode == 404:
			code = ExitNotFound
		case apiErr.StatusCode == 409:
			code = ExitConflict
		case apiErr.StatusCode == 400 || apiErr.StatusCode == 422:
			code = ExitValidation
		case apiErr.StatusCode == 429 || apiErr.StatusCode >= 500:
			code = ExitNetwork
		}
		return code, payload
	}

	if core.IsTransport(err) {
		return ExitNetwork, output.ErrorPayload{
			Code:      "network",
			Message:   err.Error(),
			Retryable: true,
			Hint:      "check the server URL and your connection (run `mealie doctor`)",
		}
	}

	return ExitError, output.ErrorPayload{Code: "error", Message: err.Error()}
}
