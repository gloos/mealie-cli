package cli

// Exit codes form part of the agent contract. They are stable and documented in
// the README so scripts can branch on failure class without parsing text.
const (
	ExitOK         = 0 // success
	ExitError      = 1 // generic/unexpected failure
	ExitUsage      = 2 // invalid flags or arguments
	ExitConfig     = 3 // configuration or authentication problem
	ExitNotFound   = 4 // requested resource does not exist
	ExitConflict   = 5 // resource conflict (e.g. already exists)
	ExitValidation = 6 // request rejected by server validation
	ExitNetwork    = 7 // network failure / transient server error after retries
)
