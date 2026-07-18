//go:build !windows

package psadt

import (
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// ssBatteryInfo is unavailable off Windows.
func ssBatteryInfo() (BatteryInfo, error) {
	return BatteryInfo{}, winerr.ErrNotWindows
}

// ssOobeCompleted is unavailable off Windows.
func ssOobeCompleted() (bool, error) {
	return false, winerr.ErrNotWindows
}

// ssPowerPointActive is unavailable off Windows.
func ssPowerPointActive() (bool, error) {
	return false, winerr.ErrNotWindows
}

// ssWwahostRunning is unavailable off Windows.
func ssWwahostRunning() (bool, error) {
	return false, winerr.ErrNotWindows
}

// ssToastNotificationMode is unavailable off Windows.
func ssToastNotificationMode() (ToastNotificationMode, bool, error) {
	return ToastNotificationModeUnrestricted, false, winerr.ErrNotWindows
}

// ssBootTime returns the zero time off Windows (no uptime source).
func ssBootTime() time.Time {
	return time.Time{}
}
