//go:build !windows

package winadt

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// newDialogServer is unavailable off Windows; the facade treats the resulting
// error as "no interactive session" and proceeds in a silent-equivalent path.
func newDialogServer(_ context.Context, _ *DeploymentSession) (*dialogserver.DialogServer, error) {
	return nil, winerr.Wrap("adt: dialogs require Windows", winerr.ErrNotWindows)
}

func initiateSystemRestart(_ context.Context, _ int, _ string) error {
	return winerr.Wrap("adt: system restart requires Windows", winerr.ErrNotWindows)
}

func blockAppExecution(_ context.Context, _ *DeploymentSession, _ []string) error {
	return winerr.Wrap("adt: application blocking requires Windows", winerr.ErrNotWindows)
}

func unblockAppExecution(_ context.Context, _ *DeploymentSession) error {
	return winerr.Wrap("adt: application blocking requires Windows", winerr.ErrNotWindows)
}

func queryUserNotificationState() (int, error) {
	return 0, winerr.Wrap("adt: user notification state requires Windows", winerr.ErrNotWindows)
}
