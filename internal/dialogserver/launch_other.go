//go:build !windows

package dialogserver

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Launch is unavailable off Windows: the re-exec path depends on
// CreateProcessAsUser and inheritable anonymous pipes.
func Launch(_ context.Context, _ LaunchConfig) (*DialogServer, error) {
	return nil, winerr.Wrap("dialogserver: re-exec launch", winerr.ErrNotWindows)
}
