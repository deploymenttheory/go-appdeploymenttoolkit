package psadt

import (
	"fmt"
	"regexp"
	"time"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/power"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/setupandmigration"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/systeminformation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
)

// Sentinel bytes/words used by SYSTEM_POWER_STATUS.
const (
	batteryFlagNoSystemBattery byte   = 128
	batteryFlagUnknown         byte   = 255
	batteryLifeUnknown         uint32 = 0xFFFFFFFF
	batteryPercentUnknown      byte   = 255
)

// powerPointTitleRe matches PowerPoint slide-show window titles (including the
// non-English "PowerPoint-" prefix), mirroring Test-ADTPowerPoint.
var powerPointTitleRe = regexp.MustCompile(`^PowerPoint(-| Slide Show)`)

// ssBatteryInfo reads SYSTEM_POWER_STATUS via GetSystemPowerStatus.
func ssBatteryInfo() (BatteryInfo, error) {
	var status power.SYSTEM_POWER_STATUS
	if err := power.GetSystemPowerStatus(&status); err != nil {
		return BatteryInfo{}, fmt.Errorf("psadt: GetSystemPowerStatus: %w", err)
	}

	invalid := status.BatteryFlag == batteryFlagUnknown || status.BatteryFlag&batteryFlagNoSystemBattery != 0

	info := BatteryInfo{
		ACPowerLineStatus:   PowerLineStatus(status.ACLineStatus),
		BatteryChargeStatus: status.BatteryFlag,
		BatteryLifePercent:  -1,
		BatterySaverEnabled: status.SystemStatusFlag == 1,
		IsUsingACPower:      (invalid && status.ACLineStatus == byte(PowerLineStatusUnknown)) || status.ACLineStatus == byte(PowerLineStatusOnline),
		IsLaptop:            ssIsLaptop(status.BatteryFlag),
	}
	if !invalid && status.BatteryLifePercent != batteryPercentUnknown {
		info.BatteryLifePercent = float64(status.BatteryLifePercent)
	}
	if status.BatteryLifeTime != batteryLifeUnknown {
		info.BatteryLifeRemaining = time.Duration(status.BatteryLifeTime) * time.Second
	}
	if status.BatteryFullLifeTime != batteryLifeUnknown {
		info.BatteryFullLifetime = time.Duration(status.BatteryFullLifeTime) * time.Second
	}
	return info, nil
}

// ssIsLaptop determines whether the machine is portable.
//
// Deviation from PSADT: the PowerShell/C# path reads the SMBIOS system
// enclosure chassis type (via WMI). WMI is unavailable here, so this uses the
// power subsystem's capabilities (a lid or system battery indicates a
// portable device), falling back to "has a system battery" from the power
// status flag when GetPwrCapabilities fails.
func ssIsLaptop(batteryFlag byte) bool {
	var caps power.SYSTEM_POWER_CAPABILITIES
	if _, err := power.GetPwrCapabilities(&caps); err != nil {
		return batteryFlag&batteryFlagNoSystemBattery == 0 && batteryFlag != batteryFlagUnknown
	}
	return caps.LidPresent != 0 || caps.SystemBatteriesPresent != 0
}

// ssOobeCompleted calls kernel32!OOBEComplete.
func ssOobeCompleted() (bool, error) {
	var complete foundation.BOOL
	if err := setupandmigration.OOBEComplete(&complete); err != nil {
		return false, fmt.Errorf("psadt: OOBEComplete: %w", err)
	}
	return complete != 0, nil
}

// ssPowerPointActive replicates Test-ADTPowerPoint's detection.
func ssPowerPointActive() (bool, error) {
	procs, err := procmgmt.RunningProcesses([]procmgmt.ProcessSpec{{Name: "POWERPNT"}})
	if err != nil {
		return false, err
	}
	if len(procs) == 0 {
		logToSession("There is no instance of PowerPoint running on this system.", LogSeverityInfo, "TestADTPowerPoint")
		return false, nil
	}

	pids := make(map[uint32]bool, len(procs))
	for _, p := range procs {
		pids[p.PID] = true
	}
	windows, err := procmgmt.WindowTitles()
	if err != nil {
		return false, err
	}
	for _, w := range windows {
		if pids[w.PID] && powerPointTitleRe.MatchString(w.Title) {
			logToSession("Detected a PowerPoint process with a window title indicating a slide show is active.", LogSeverityInfo, "TestADTPowerPoint")
			return true, nil
		}
	}

	state, err := ssUserNotificationState()
	if err != nil {
		return false, err
	}
	switch state {
	case shell.QUNS_PRESENTATION_MODE:
		logToSession("Detected the user's notification state is presentation mode.", LogSeverityInfo, "TestADTPowerPoint")
		return true, nil
	case shell.QUNS_BUSY:
		logToSession("Detected the user's notification state is busy.", LogSeverityInfo, "TestADTPowerPoint")
		return true, nil
	}
	logToSession("Unable to detect any indication of an ongoing presentation.", LogSeverityInfo, "TestADTPowerPoint")
	return false, nil
}

// ssWwahostRunning reports whether a wwahost process (the ESP host) is running.
func ssWwahostRunning() (bool, error) {
	procs, err := procmgmt.RunningProcesses([]procmgmt.ProcessSpec{{Name: "wwahost"}})
	if err != nil {
		return false, err
	}
	return len(procs) > 0, nil
}

// ssUserNotificationState queries SHQueryUserNotificationState.
func ssUserNotificationState() (shell.QUERY_USER_NOTIFICATION_STATE, error) {
	var state shell.QUERY_USER_NOTIFICATION_STATE
	if err := shell.SHQueryUserNotificationState(&state); err != nil {
		return 0, fmt.Errorf("psadt: SHQueryUserNotificationState: %w", err)
	}
	return state, nil
}

// ssToastNotificationMode approximates the toast notification mode from the
// shell user notification state. See GetADTUserToastNotificationMode.
func ssToastNotificationMode() (ToastNotificationMode, bool, error) {
	state, err := ssUserNotificationState()
	if err != nil {
		return ToastNotificationModeUnrestricted, false, err
	}
	switch state {
	case shell.QUNS_NOT_PRESENT:
		return ToastNotificationModeUnrestricted, false, nil
	case shell.QUNS_QUIET_TIME:
		return ToastNotificationModePriorityOnly, true, nil
	case shell.QUNS_PRESENTATION_MODE, shell.QUNS_BUSY, shell.QUNS_RUNNING_D3D_FULL_SCREEN:
		return ToastNotificationModeAlarmsOnly, true, nil
	default:
		return ToastNotificationModeUnrestricted, true, nil
	}
}

// ssBootTime derives the system boot time from the tick count (uptime).
func ssBootTime() time.Time {
	uptime := time.Duration(systeminformation.GetTickCount64()) * time.Millisecond //#nosec G115 -- uptime milliseconds fit a Duration for any realistic uptime
	return time.Now().Add(-uptime)
}
