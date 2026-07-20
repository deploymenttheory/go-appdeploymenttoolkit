package procmgmt

import (
	"errors"
	"fmt"
	"time"

	"golang.org/x/sys/windows"
)

// MutexAvailable ports Test-ADTMutexAvailability: it opens the named system
// mutex (e.g. `Global\_MSIExecute`) without acquiring it, then waits up to
// wait for an exclusive lock, releasing it immediately when acquired.
//
// Outcomes mirror the PowerShell catch blocks: a missing mutex is available
// (true); access denied means it exists but is unusable (false); an
// abandoned mutex counts as acquired (true); any other failure errs on the
// side of available (true) with the error returned for the caller to log.
// A negative wait blocks indefinitely; zero tests the state and returns
// immediately.
func MutexAvailable(name string, wait time.Duration) (bool, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return true, fmt.Errorf("procmgmt: invalid mutex name %q: %w", name, err)
	}

	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, namePtr)
	if err != nil {
		switch {
		case errors.Is(err, windows.ERROR_FILE_NOT_FOUND):
			// WaitHandleCannotBeOpenedException: the mutex does not exist.
			return true, nil
		case errors.Is(err, windows.ERROR_ACCESS_DENIED):
			// UnauthorizedAccessException: exists but inaccessible.
			return false, nil
		default:
			return true, fmt.Errorf("procmgmt: OpenMutex(%q): %w", name, err)
		}
	}
	defer func() { _ = windows.CloseHandle(h) }()

	waitMs := uint32(windows.INFINITE)
	if wait >= 0 {
		waitMs = uint32(wait.Milliseconds()) //#nosec G115 -- caller-bounded wait duration
	}
	event, err := windows.WaitForSingleObject(h, waitMs)
	if err != nil {
		return true, fmt.Errorf("procmgmt: WaitForSingleObject(%q): %w", name, err)
	}
	switch event {
	case windows.WAIT_OBJECT_0, uint32(windows.WAIT_ABANDONED):
		// Acquired (an abandoned mutex transfers ownership); release the
		// lock we just took so the probe has no side effects.
		_ = windows.ReleaseMutex(h)
		return true, nil
	default: // WAIT_TIMEOUT
		return false, nil
	}
}
