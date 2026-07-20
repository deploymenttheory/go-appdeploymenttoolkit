package configmgr

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
	wmi "github.com/deploymenttheory/go-bindings-wmi/runtime/wmi"
)

// Trigger invokes ROOT\CCM SMS_Client.TriggerSchedule for the given schedule,
// mirroring PSADT's Invoke-ADTSCCMTask (which calls Invoke-CimMethod). It uses
// the typed WMI runtime to pass the single sScheduleID GUID argument.
//
// This is a thin marshaling layer that cannot be exercised off Windows; see
// docs/windows-smoke.md for the verification step.
func Trigger(ctx context.Context, id ScheduleID) error {
	svc, err := wmi.Connect(`root\ccm`)
	if err != nil {
		return fmt.Errorf(
			"configmgr: connecting to root\\ccm (is the ConfigMgr client installed?): %w",
			err,
		)
	}
	defer svc.Close()

	row, err := svc.ExecMethodContext(ctx, "SMS_Client", "TriggerSchedule", map[string]any{
		"sScheduleID": id.ScheduleGUID(),
	})
	if err != nil {
		return fmt.Errorf("configmgr: TriggerSchedule for %s: %w", id.ScheduleGUID(), err)
	}
	if rv, ok := rowInt(row, "ReturnValue"); ok && rv != 0 {
		return winerr.Wrap(
			fmt.Sprintf("configmgr: TriggerSchedule returned error code %d", rv),
			winerr.ErrInvalidOption,
		)
	}
	return nil
}

// IsHotfixInstalled reports whether a Windows update (KB) is installed,
// querying Win32_QuickFixEngineering — the Get-Hotfix equivalent PSADT's
// Test-ADTMSUpdates uses.
func IsHotfixInstalled(ctx context.Context, kbNumber string) (bool, error) {
	svc, err := wmi.Connect(`root\cimv2`)
	if err != nil {
		return false, fmt.Errorf("configmgr: connecting to root\\cimv2: %w", err)
	}
	defer svc.Close()

	rows, err := svc.QueryContext(
		ctx,
		fmt.Sprintf(
			"SELECT HotFixID FROM Win32_QuickFixEngineering WHERE HotFixID = '%s'",
			sanitizeKB(kbNumber),
		),
	)
	if err != nil {
		return false, fmt.Errorf("configmgr: querying hotfixes: %w", err)
	}
	return len(rows) > 0, nil
}

func rowInt(row wmi.Row, key string) (int64, bool) {
	switch v := row[key].(type) {
	case int64:
		return v, true
	case int32:
		return int64(v), true
	case int:
		return int64(v), true
	case uint32:
		return int64(v), true
	default:
		return 0, false
	}
}

// sanitizeKB keeps only the KB identifier's alphanumerics so it is safe to
// embed in the WQL string literal (KB IDs are alphanumeric, e.g. "KB2549864").
func sanitizeKB(kb string) string {
	out := make([]rune, 0, len(kb))
	for _, r := range kb {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			out = append(out, r)
		}
	}
	return string(out)
}
