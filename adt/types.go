package adt

import (
	"errors"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// DeploymentType mirrors PSADT's Install/Uninstall/Repair verbs.
type DeploymentType = session.DeploymentType

// DeploymentType values.
const (
	DeploymentTypeInstall   = session.DeploymentTypeInstall
	DeploymentTypeUninstall = session.DeploymentTypeUninstall
	DeploymentTypeRepair    = session.DeploymentTypeRepair
)

// DeployMode mirrors PSADT's Auto/Interactive/NonInteractive/Silent modes.
type DeployMode = session.DeployMode

// DeployMode values.
const (
	DeployModeAuto           = session.DeployModeAuto
	DeployModeInteractive    = session.DeployModeInteractive
	DeployModeNonInteractive = session.DeployModeNonInteractive
	DeployModeSilent         = session.DeployModeSilent
)

// ProcessObject mirrors PSADT's process-to-close descriptor
// (the AppProcessesToClose entries of Invoke-AppDeployToolkit.ps1).
type ProcessObject = session.ProcessObject

// Sentinel errors surfaced by the toolkit. Match with errors.Is.
var (
	ErrNoActiveSession   = winerr.ErrNoActiveSession
	ErrDeferred          = winerr.ErrDeferred
	ErrUserCancelled     = winerr.ErrUserCancelled
	ErrTimeout           = winerr.ErrTimeout
	ErrNotFound          = winerr.ErrNotFound
	ErrNotImplemented    = winerr.ErrNotImplemented
	ErrInvalidOption     = winerr.ErrInvalidOption
	ErrDialogUnavailable = winerr.ErrDialogUnavailable
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
