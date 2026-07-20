package winadt

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/configmgr"
)

// InvokeADTSCCMTask is the Go port of Invoke-ADTSCCMTask: it triggers a
// Configuration Manager (SCCM/MECM) client schedule task by its name (for
// example "SoftwareUpdatesScan" or "HardwareInventory"), via the ROOT\CCM
// SMS_Client.TriggerSchedule WMI method.
func InvokeADTSCCMTask(ctx context.Context, scheduleID string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	id, err := configmgr.ParseScheduleID(scheduleID)
	if err != nil {
		return err
	}
	logSessionInfo(fmt.Sprintf("Triggering SCCM task [%s].", scheduleID), "InvokeADTSCCMTask")
	return configmgr.Trigger(ctx, id)
}

// InstallADTSCCMSoftwareUpdates is the Go port of
// Install-ADTSCCMSoftwareUpdates: it triggers the ConfigMgr software-updates
// scan and deployment-evaluation cycles so pending updates install.
func InstallADTSCCMSoftwareUpdates(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	logSessionInfo("Triggering SCCM software update scan and evaluation.", "InstallADTSCCMSoftwareUpdates")
	for _, task := range []string{"SoftwareUpdatesScan", "SoftwareUpdatesAgentAssignmentEvaluation"} {
		if err := InvokeADTSCCMTask(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

// TestADTMSUpdates is the Go port of Test-ADTMSUpdates: it reports whether the
// given Windows update (KB number, e.g. "KB2549864") is installed, querying
// Win32_QuickFixEngineering — the Get-Hotfix source PSADT uses.
func TestADTMSUpdates(ctx context.Context, kbNumber string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	kb := normalizeKB(kbNumber)
	if kb == "" {
		return false, nil
	}
	return configmgr.IsHotfixInstalled(ctx, kb)
}

// normalizeKB upper-cases and ensures the "KB" prefix on a KB identifier
// (Win32_QuickFixEngineering's HotFixID is stored as "KB…").
func normalizeKB(kb string) string {
	kb = strings.ToUpper(strings.TrimSpace(kb))
	if kb == "" {
		return ""
	}
	if !strings.HasPrefix(kb, "KB") {
		kb = "KB" + kb
	}
	return kb
}
