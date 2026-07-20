// Package errs hosts the toolkit's platform-neutral sentinel error catalog
// and the Wrap annotation helper. The Windows result-code converters
// (WIN32_ERROR, MSI, HRESULT) live in internal/win/winerr, which re-exports
// these sentinels for its consumers.
package errs

import (
	"errors"
	"fmt"
)

// Sentinel errors used across the toolkit. Callers match them with errors.Is.
var (
	// ErrNotImplemented marks a function that has not been ported yet.
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

// Wrap annotates err with the toolkit operation name, preserving the chain
// for errors.Is/As. Returns nil when err is nil.
func Wrap(op string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("%s: %w", op, err)
}
