package adt

import (
	"context"
	"fmt"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/svcmgmt"
)

// ServiceStartMode mirrors PSADT's service start-mode names
// ("Automatic", "Automatic (Delayed Start)", "Manual", "Disabled").
type ServiceStartMode = svcmgmt.StartMode

// ServiceStartMode values.
const (
	ServiceStartModeAutomatic        = svcmgmt.StartModeAutomatic
	ServiceStartModeAutomaticDelayed = svcmgmt.StartModeAutomaticDelayed
	ServiceStartModeManual           = svcmgmt.StartModeManual
	ServiceStartModeDisabled         = svcmgmt.StartModeDisabled
)

const defaultServiceTimeout = 60 * time.Second

// TestADTServiceExists is the Go port of Test-ADTServiceExists: it reports
// whether the named Windows service is installed.
func TestADTServiceExists(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	return svcmgmt.Exists(name)
}

// GetADTServiceStartMode is the Go port of Get-ADTServiceStartMode.
func GetADTServiceStartMode(ctx context.Context, name string) (ServiceStartMode, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	return svcmgmt.GetStartMode(name)
}

// SetADTServiceStartMode is the Go port of Set-ADTServiceStartMode.
func SetADTServiceStartMode(ctx context.Context, name string, mode ServiceStartMode) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	logSessionInfo(fmt.Sprintf("Setting service [%s] start mode to [%s].", name, mode), "SetADTServiceStartMode")
	return svcmgmt.SetStartMode(name, mode)
}

// StartADTServiceAndDependenciesOptions configures the service start/stop
// functions.
type StartADTServiceAndDependenciesOptions struct {
	// PendingStatusWait bounds how long to wait for the target state.
	// Zero means 60 seconds (PSADT default).
	PendingStatusWait time.Duration
}

// StartADTServiceAndDependencies is the Go port of
// Start-ADTServiceAndDependencies: it starts the service, waits for it to
// run, then starts its dependent services.
func StartADTServiceAndDependencies(ctx context.Context, name string, opts ...StartADTServiceAndDependenciesOptions) error {
	timeout := defaultServiceTimeout
	if len(opts) > 0 && opts[0].PendingStatusWait > 0 {
		timeout = opts[0].PendingStatusWait
	}
	logSessionInfo(fmt.Sprintf("Starting service [%s] and its dependencies.", name), "StartADTServiceAndDependencies")
	return svcmgmt.StartWithDependencies(ctx, name, timeout)
}

// StopADTServiceAndDependencies is the Go port of
// Stop-ADTServiceAndDependencies: it stops the service's running dependents
// first, then the service itself.
func StopADTServiceAndDependencies(ctx context.Context, name string, opts ...StartADTServiceAndDependenciesOptions) error {
	timeout := defaultServiceTimeout
	if len(opts) > 0 && opts[0].PendingStatusWait > 0 {
		timeout = opts[0].PendingStatusWait
	}
	logSessionInfo(fmt.Sprintf("Stopping service [%s] and its dependencies.", name), "StopADTServiceAndDependencies")
	return svcmgmt.Stop(ctx, name, true, timeout)
}

// logSessionInfo logs through the active session when one exists; toolkit
// functions work sessionless and simply skip logging otherwise.
func logSessionInfo(message, source string) {
	if s, err := GetADTSession(); err == nil {
		s.WriteLog(message, LogSeverityInfo, source, "")
	}
}
