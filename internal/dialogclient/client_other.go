//go:build !windows

package dialogclient

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// ClientMain is the `client` subcommand entrypoint. Off Windows there is no
// interactive session UI to host, so it reports ErrNotWindows.
func ClientMain(_ context.Context, _ Config) error {
	return winerr.Wrap("dialogclient: client mode requires Windows", winerr.ErrNotWindows)
}
