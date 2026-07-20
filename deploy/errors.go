package deploy

import (
	"errors"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// Sentinel errors surfaced by the toolkit. Match with errors.Is.
var (
	ErrNoActiveSession   = errs.ErrNoActiveSession
	ErrDeferred          = errs.ErrDeferred
	ErrUserCancelled     = errs.ErrUserCancelled
	ErrTimeout           = errs.ErrTimeout
	ErrNotFound          = errs.ErrNotFound
	ErrNotImplemented    = errs.ErrNotImplemented
	ErrInvalidOption     = errs.ErrInvalidOption
	ErrDialogUnavailable = errs.ErrDialogUnavailable
)

// Reserved exit codes carried over from PSADT.
const (
	ExitCodeGenericFailure = 60001 // unhandled deployment failure
	ExitCodeRunnerFailure  = 60008 // frontend/runner fatal error
	ExitCodeUserDeferral   = 60012 // legacy defer code (config UI.DeferExitCode wins)
	ExitCodeUserCancel     = 1602  // user cancelled (also default DeferExitCode)
	ExitCodeDialogTimeout  = 1618  // default UI timeout exit code
	ExitCodeRebootRequired = 3010
	ExitCodeHardReboot     = 1641
)

// ExitError requests a specific process exit code from a phase function.
// Return it (or wrap it) from a PhaseFunc to control the deployment's exit
// code; the Deployment runner unwraps it with errors.As.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string {
	if e.Err == nil {
		return fmt.Sprintf("adt: exit with code %d", e.Code)
	}
	return fmt.Sprintf("adt: exit with code %d: %v", e.Code, e.Err)
}

// Unwrap exposes the wrapped cause.
func (e *ExitError) Unwrap() error { return e.Err }

// NewExitError builds an ExitError with an optional cause.
func NewExitError(code int, err error) *ExitError {
	return &ExitError{Code: code, Err: err}
}

// AsExitError extracts an ExitError from a chain, if present.
func AsExitError(err error) (*ExitError, bool) {
	var ee *ExitError
	ok := errors.As(err, &ee)
	return ee, ok
}
