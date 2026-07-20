//go:build !windows

package dialogserver

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// Launch is unavailable off Windows: the re-exec path depends on
// CreateProcessAsUser and inheritable anonymous pipes.
func Launch(_ context.Context, _ LaunchConfig) (*DialogServer, error) {
	return nil, errs.Wrap("dialogserver: re-exec launch", errs.ErrNotWindows)
}
