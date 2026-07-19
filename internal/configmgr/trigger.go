package configmgr

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Trigger invokes ROOT\CCM SMS_Client.TriggerSchedule for the given schedule,
// mirroring PSADT's Invoke-ADTSCCMTask.
//
// The WMI method-invocation path is not yet wired: it requires a WMI runtime,
// and go-bindings-wmi is currently incompatible with the pinned
// go-bindings-win32 release. The schedule-ID resolution and GUID composition
// (the ConfigMgr-specific logic) are complete and tested; only the final
// SMS_Client.TriggerSchedule call returns ErrNotImplemented. See
// docs/windows-smoke.md.
func Trigger(_ context.Context, id ScheduleID) error {
	return fmt.Errorf(
		"configmgr: TriggerSchedule(%s): %w",
		id.ScheduleGUID(),
		winerr.ErrNotImplemented,
	)
}
