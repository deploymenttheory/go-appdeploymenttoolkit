//go:build !windows

// Package svcmgmt wraps the Windows Service Control Manager operations the
// toolkit needs. On non-Windows platforms every operation returns
// winerr.ErrNotWindows.
package svcmgmt

import (
	"context"
	"strings"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// StartMode mirrors PSADT's service start-mode names.
type StartMode string

// StartMode values.
const (
	StartModeAutomatic        StartMode = "Automatic"
	StartModeAutomaticDelayed StartMode = "Automatic (Delayed Start)"
	StartModeManual           StartMode = "Manual"
	StartModeDisabled         StartMode = "Disabled"
)

// ParseStartMode validates a PSADT start-mode string.
func ParseStartMode(s string) (StartMode, error) {
	for _, m := range []StartMode{StartModeAutomatic, StartModeAutomaticDelayed, StartModeManual, StartModeDisabled} {
		if strings.EqualFold(string(m), s) {
			return m, nil
		}
	}
	return "", winerr.Wrap("svcmgmt: start mode "+s, winerr.ErrInvalidOption)
}

// Exists reports whether the named service is installed.
func Exists(string) (bool, error) { return false, winerr.ErrNotWindows }

// GetStartMode returns the service's start mode.
func GetStartMode(string) (StartMode, error) { return "", winerr.ErrNotWindows }

// SetStartMode sets the service start mode.
func SetStartMode(string, StartMode) error { return winerr.ErrNotWindows }

// Start starts the named service.
func Start(context.Context, string, time.Duration) error { return winerr.ErrNotWindows }

// Stop stops the named service.
func Stop(context.Context, string, bool, time.Duration) error { return winerr.ErrNotWindows }

// StartWithDependencies starts the service and its dependents.
func StartWithDependencies(context.Context, string, time.Duration) error {
	return winerr.ErrNotWindows
}
