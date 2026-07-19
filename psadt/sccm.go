package psadt

import (
	"context"
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/configmgr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

// InvokeADTSCCMTask is the Go port of Invoke-ADTSCCMTask: it triggers a
// Configuration Manager (SCCM/MECM) client schedule task by its name (for
// example "SoftwareUpdatesScan" or "HardwareInventory"), via the ROOT\CCM
// SMS_Client.TriggerSchedule WMI method.
func InvokeADTSCCMTask(ctx context.Context, scheduleID string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: %w", err)
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
		return fmt.Errorf("psadt: %w", err)
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
// given Windows update (KB number, e.g. "KB2549864") is installed.
//
// Deviation from PSADT: PSADT queries Win32_QuickFixEngineering (Get-Hotfix)
// with a Windows Update Agent COM fallback. To avoid a WMI dependency this
// port scans the Component Based Servicing package list in the registry,
// which lists a superset of the servicing-installed KBs.
func TestADTMSUpdates(ctx context.Context, kbNumber string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("psadt: %w", err)
	}
	return hotfixInstalled(registryBackend(), kbNumber)
}

const cbsPackagesKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\Packages`

// hotfixInstalled reports whether any CBS package name references the KB. It
// takes a registry backend so the matching logic is testable with a fake.
func hotfixInstalled(backend regkey.Backend, kbNumber string) (bool, error) {
	kb := normalizeKB(kbNumber)
	if kb == "" {
		return false, nil
	}
	packages, err := backend.EnumSubkeys("HKLM", cbsPackagesKey)
	if err != nil {
		return false, fmt.Errorf("psadt: enumerating servicing packages: %w", err)
	}
	for _, pkg := range packages {
		if strings.Contains(strings.ToUpper(pkg), kb) {
			return true, nil
		}
	}
	return false, nil
}

// normalizeKB upper-cases and ensures the "KB" prefix on a KB identifier.
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
