//go:build !windows

package configmgr

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// Trigger requires WMI and the ConfigMgr client; unavailable off Windows.
func Trigger(context.Context, ScheduleID) error { return winerr.ErrNotWindows }

// IsHotfixInstalled requires WMI; unavailable off Windows.
func IsHotfixInstalled(context.Context, string) (bool, error) { return false, winerr.ErrNotWindows }
