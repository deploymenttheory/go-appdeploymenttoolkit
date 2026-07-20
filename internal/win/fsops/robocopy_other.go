//go:build !windows

package fsops

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// copyRobocopy is Windows-only; other platforms cannot shell out to robocopy.
func copyRobocopy(_ context.Context, _ []string, _ string, _ CopyOptions) error {
	return winerr.Wrap("fsops: robocopy copy mode", winerr.ErrNotWindows)
}
