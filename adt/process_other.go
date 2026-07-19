//go:build !windows

package adt

import (
	"context"
	"fmt"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
)

// Non-Windows builds (tests and tooling): launching works portably via
// os/exec; the WTS/mutex/window functions surface ErrNotWindows.

// processLauncher is the launcher behind StartADTProcess off-Windows.
var processLauncher procmgmt.Launcher = procmgmt.ExecLauncher{}

// mutexAvailable reports "available" so portable StartADTProcess flows are
// not blocked, with the sentinel attached for callers that care.
func mutexAvailable(name string, _ time.Duration) (bool, error) {
	return true, fmt.Errorf("adt: probing mutex %q: %w", name, errNotWindows)
}

func runningProcesses(_ []procmgmt.ProcessSpec) ([]RunningProcess, error) {
	return nil, fmt.Errorf("adt: GetADTRunningProcesses: %w", errNotWindows)
}

func windowTitles() ([]WindowInfo, error) {
	return nil, fmt.Errorf("adt: GetADTWindowTitle: %w", errNotWindows)
}

func startADTProcessAsUser(
	_ context.Context,
	_ StartADTProcessAsUserOptions,
) (*ProcessResult, error) {
	return nil, fmt.Errorf("adt: StartADTProcessAsUser: %w", errNotWindows)
}
