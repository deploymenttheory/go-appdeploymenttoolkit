package winadt

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
)

// This file re-exports the shared deployment engine (package deploy) under
// the PSADT-parity names. Every type is an identical alias, so values flow
// between the two packages without conversion; existing deployments only
// ever change their import path.

// DeploymentSession is the deployment session (deploy.Session).
type DeploymentSession = deploy.Session

// SessionOptions mirrors the $adtSession hashtable passed to Open-ADTSession.
type SessionOptions = deploy.SessionOptions

// SessionHooks carries the module callback stages (deploy.Hooks).
type SessionHooks = deploy.Hooks

// PhaseFunc is one deployment phase.
type PhaseFunc = deploy.PhaseFunc

// Deployment is the Go analogue of an Invoke-AppDeployToolkit.ps1 frontend
// script: session metadata plus the phase functions per deployment type.
type Deployment = deploy.Deployment

// DeploymentType mirrors PSADT's Install/Uninstall/Repair verbs.
type DeploymentType = deploy.DeploymentType

// DeploymentType values.
const (
	DeploymentTypeInstall   = deploy.DeploymentTypeInstall
	DeploymentTypeUninstall = deploy.DeploymentTypeUninstall
	DeploymentTypeRepair    = deploy.DeploymentTypeRepair
)

// DeployMode mirrors PSADT's Auto/Interactive/NonInteractive/Silent modes.
type DeployMode = deploy.DeployMode

// DeployMode values.
const (
	DeployModeAuto           = deploy.DeployModeAuto
	DeployModeInteractive    = deploy.DeployModeInteractive
	DeployModeNonInteractive = deploy.DeployModeNonInteractive
	DeployModeSilent         = deploy.DeployModeSilent
)

// ProcessObject mirrors PSADT's process-to-close descriptor.
type ProcessObject = deploy.ProcessObject

// ExitError requests a specific process exit code from a phase function.
type ExitError = deploy.ExitError

// Sentinel errors surfaced by the toolkit. Match with errors.Is.
var (
	ErrNoActiveSession   = deploy.ErrNoActiveSession
	ErrDeferred          = deploy.ErrDeferred
	ErrUserCancelled     = deploy.ErrUserCancelled
	ErrTimeout           = deploy.ErrTimeout
	ErrNotFound          = deploy.ErrNotFound
	ErrNotImplemented    = deploy.ErrNotImplemented
	ErrInvalidOption     = deploy.ErrInvalidOption
	ErrDialogUnavailable = deploy.ErrDialogUnavailable
)

// Reserved exit codes carried over from PSADT.
const (
	ExitCodeGenericFailure = deploy.ExitCodeGenericFailure
	ExitCodeRunnerFailure  = deploy.ExitCodeRunnerFailure
	ExitCodeUserDeferral   = deploy.ExitCodeUserDeferral
	ExitCodeUserCancel     = deploy.ExitCodeUserCancel
	ExitCodeDialogTimeout  = deploy.ExitCodeDialogTimeout
	ExitCodeRebootRequired = deploy.ExitCodeRebootRequired
	ExitCodeHardReboot     = deploy.ExitCodeHardReboot
)

// LogSeverity mirrors PSADT's LogSeverity enum.
type LogSeverity = deploy.LogSeverity

// LogSeverity values.
const (
	LogSeveritySuccess = deploy.LogSeveritySuccess
	LogSeverityInfo    = deploy.LogSeverityInfo
	LogSeverityWarning = deploy.LogSeverityWarning
	LogSeverityError   = deploy.LogSeverityError
)

// LogEntry is one rendered toolkit log entry (SessionHooks.OnLogEntry).
type LogEntry = deploy.LogEntry

// LogEntryOptions mirrors the parameters of Write-ADTLogEntry.
type LogEntryOptions = deploy.LogEntryOptions

// NewExitError builds an ExitError with an optional cause.
func NewExitError(code int, err error) *ExitError { return deploy.NewExitError(code, err) }

// AsExitError extracts an ExitError from a chain, if present.
func AsExitError(err error) (*ExitError, bool) { return deploy.AsExitError(err) }

// OpenADTSession is the Go port of Open-ADTSession.
func OpenADTSession(ctx context.Context, opts SessionOptions) (*DeploymentSession, error) {
	return deploy.Open(ctx, opts)
}

// CloseADTSession is the Go port of Close-ADTSession.
func CloseADTSession(ctx context.Context, s *DeploymentSession) int {
	return deploy.Close(ctx, s)
}

// GetADTSession is the Go port of Get-ADTSession: it returns the most
// recently opened active session.
func GetADTSession() (*DeploymentSession, error) { return deploy.Current() }

// TestADTSessionActive is the Go port of Test-ADTSessionActive.
func TestADTSessionActive() bool { return deploy.Active() }

// TestADTCallerIsAdmin is the Go port of Test-ADTCallerIsAdmin.
func TestADTCallerIsAdmin() bool { return deploy.CallerIsAdmin() }

// AddADTSessionClosingCallback appends a Closing hook to an open session.
func AddADTSessionClosingCallback(
	s *DeploymentSession,
	fn func(ctx context.Context, s *DeploymentSession) error,
) {
	deploy.AddClosingHook(s, fn)
}

// WriteADTLogEntry is the Go port of Write-ADTLogEntry.
//
// This MUST stay a var alias (not a wrapper func): Source defaulting
// resolves the caller a fixed number of frames up, and a wrapper would add
// a frame and misattribute every entry to this package.
var WriteADTLogEntry = deploy.WriteLogEntry
