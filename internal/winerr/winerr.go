// Package winerr converts raw Windows result codes (WIN32_ERROR values,
// Windows Installer uint32 return codes, and COM HRESULTs) into wrapped Go
// errors, and hosts the toolkit's sentinel error catalog.
//
// The package is deliberately free of any Windows-only imports so that error
// classification logic is unit-testable on every platform.
package winerr

import (
	"errors"
	"fmt"
)

// Sentinel errors used across the toolkit. Callers match them with errors.Is.
var (
	// ErrNotImplemented marks a PSADT function that has not been ported yet.
	ErrNotImplemented = errors.New("adt: not implemented")

	// ErrNoActiveSession is returned when no deployment session is open.
	ErrNoActiveSession = errors.New("adt: no active deployment session")

	// ErrSessionClosed is returned when operating on a closed session.
	ErrSessionClosed = errors.New("adt: deployment session already closed")

	// ErrDeferred signals that the user chose to defer the deployment.
	ErrDeferred = errors.New("adt: deployment deferred by user")

	// ErrUserCancelled signals that the user cancelled a dialog or the
	// welcome-prompt countdown was aborted.
	ErrUserCancelled = errors.New("adt: cancelled by user")

	// ErrTimeout signals that a dialog or wait operation timed out.
	ErrTimeout = errors.New("adt: operation timed out")

	// ErrNotWindows is returned by Windows-only operations invoked on a
	// non-Windows platform (only possible in tests and tooling).
	ErrNotWindows = errors.New("adt: operation requires Windows")

	// ErrNotFound is a generic not-found condition (registry value, file,
	// service, application...) distinct from a syscall failure.
	ErrNotFound = errors.New("adt: not found")

	// ErrInvalidOption reports invalid or conflicting option values passed
	// to a public toolkit function.
	ErrInvalidOption = errors.New("adt: invalid option")

	// ErrDialogUnavailable is returned when no dialog can be shown (no user
	// session reachable and no fallback possible).
	ErrDialogUnavailable = errors.New("adt: dialog unavailable")
)

// Win32Error wraps a WIN32_ERROR (GetLastError-domain) code.
type Win32Error struct {
	Op   string // the API or operation that failed, e.g. "RegOpenKeyExW"
	Code uint32
}

func (e *Win32Error) Error() string {
	return fmt.Sprintf("%s: win32 error %d (0x%08X)", e.Op, e.Code, e.Code)
}

// FromWin32 converts a WIN32_ERROR-style code to an error, nil when the code
// is ERROR_SUCCESS. Use for APIs that return the error code as their value
// (registry, authorization families in go-bindings-win32).
func FromWin32(op string, code uint32) error {
	if code == 0 {
		return nil
	}
	return &Win32Error{Op: op, Code: code}
}

// MsiError wraps a Windows Installer function result (winerror/msi domain).
type MsiError struct {
	Op   string
	Code uint32
}

func (e *MsiError) Error() string {
	return fmt.Sprintf("%s: msi error %d (0x%08X)", e.Op, e.Code, e.Code)
}

// FromMsi converts an MSI uint32 return code to an error, nil on ERROR_SUCCESS.
func FromMsi(op string, code uint32) error {
	if code == 0 {
		return nil
	}
	return &MsiError{Op: op, Code: code}
}

// HResultError wraps a failed COM HRESULT.
type HResultError struct {
	Op string
	HR int32
}

func (e *HResultError) Error() string {
	return fmt.Sprintf(
		"%s: HRESULT 0x%08X",
		e.Op,
		uint32(e.HR),
	) //#nosec G115 -- reinterpreting the HRESULT bit pattern for hex display
}

// FromHResult converts an HRESULT to an error, nil when the value indicates
// success (non-negative).
func FromHResult(op string, hr int32) error {
	if hr >= 0 {
		return nil
	}
	return &HResultError{Op: op, HR: hr}
}

// Wrap annotates err with the toolkit operation name, preserving the chain
// for errors.Is/As. Returns nil when err is nil.
func Wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
