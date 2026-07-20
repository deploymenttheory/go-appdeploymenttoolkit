// System-state probes (battery, network, PowerPoint, microphone, OOBE/ESP,
// pending reboot, notification state).
//
// This file is the portable facade for the Test-ADT*/Get-ADT* system-state
// functions. Registry-driven and stdlib-driven logic lives here so it is
// exercised on every platform (and unit-testable with regkey.Fake); the thin
// Windows syscalls live in systemstate_windows.go with non-Windows stubs in
// systemstate_other.go.
package winadt

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// PowerLineStatus mirrors PSADT.DeviceManagement.PowerLineStatus (the values
// of SYSTEM_POWER_STATUS.ACLineStatus).
type PowerLineStatus byte

// PowerLineStatus values.
const (
	PowerLineStatusOffline PowerLineStatus = 0
	PowerLineStatusOnline  PowerLineStatus = 1
	PowerLineStatusUnknown PowerLineStatus = 255
)

// BatteryInfo mirrors PSADT.DeviceManagement.BatteryInfo (the object returned
// by Test-ADTBattery -PassThru).
type BatteryInfo struct {
	// ACPowerLineStatus is the raw AC line status (Online/Offline/Unknown).
	ACPowerLineStatus PowerLineStatus
	// BatteryChargeStatus is the raw SYSTEM_POWER_STATUS.BatteryFlag byte
	// (a bitmask: 1 High, 2 Low, 4 Critical, 8 Charging, 128 NoSystemBattery,
	// 255 Unknown).
	BatteryChargeStatus byte
	// BatteryLifePercent is the remaining charge as a 0-100 percentage, or -1
	// when unknown/invalid.
	BatteryLifePercent float64
	// BatterySaverEnabled reports whether battery saver is active.
	BatterySaverEnabled bool
	// BatteryLifeRemaining is the estimated remaining runtime, or 0 when
	// unknown.
	BatteryLifeRemaining time.Duration
	// BatteryFullLifetime is the estimated full-charge runtime, or 0 when
	// unknown.
	BatteryFullLifetime time.Duration
	// IsUsingACPower reports whether the machine is running on AC power.
	IsUsingACPower bool
	// IsLaptop reports whether the machine is a portable device.
	IsLaptop bool
}

// PendingRebootInfo mirrors PSADT.DeviceManagement.RebootInfo (the object
// returned by Get-ADTPendingReboot).
type PendingRebootInfo struct {
	ComputerName                 string
	LastBootUpTime               time.Time
	IsSystemRebootPending        bool
	IsCBServicingRebootPending   bool
	IsWindowsUpdateRebootPending bool
	IsSCCMClientRebootPending    bool
	IsIntuneClientRebootPending  bool
	IsAppVRebootPending          bool
	IsFileRenameRebootPending    bool
	PendingFileRenameOperations  []string
	// Reasons lists the human-readable signals that were detected.
	Reasons []string
	// ErrorMsg collects non-fatal errors encountered while probing (parity
	// with RebootInfo.ErrorMsg).
	ErrorMsg []string
}

// ToastNotificationMode approximates Windows.UI.Notifications.ToastNotificationMode.
type ToastNotificationMode int

// ToastNotificationMode values.
const (
	ToastNotificationModeUnrestricted ToastNotificationMode = 0
	ToastNotificationModePriorityOnly ToastNotificationMode = 1
	ToastNotificationModeAlarmsOnly   ToastNotificationMode = 2
)

// String renders the mode name.
func (m ToastNotificationMode) String() string {
	switch m {
	case ToastNotificationModeUnrestricted:
		return "Unrestricted"
	case ToastNotificationModePriorityOnly:
		return "PriorityOnly"
	case ToastNotificationModeAlarmsOnly:
		return "AlarmsOnly"
	default:
		return fmt.Sprintf("ToastNotificationMode(%d)", int(m))
	}
}

// PresentationUser identifies a user with presentation mode enabled.
type PresentationUser struct {
	SID       string
	NTAccount string
}

// TestADTBattery is the Go port of Test-ADTBattery: it reports the machine's
// battery and power state via GetSystemPowerStatus. The returned bool of the
// PowerShell function corresponds to BatteryInfo.IsUsingACPower.
//
// An active ADT session is NOT required.
func TestADTBattery(ctx context.Context) (BatteryInfo, error) {
	if err := ctx.Err(); err != nil {
		return BatteryInfo{}, fmt.Errorf("adt: %w", err)
	}
	logToSession("Checking if system is using AC power or if it is running on battery...", LogSeverityInfo, "TestADTBattery")
	info, err := ssBatteryInfo()
	if err != nil {
		return BatteryInfo{}, err
	}
	if info.IsUsingACPower {
		logToSession("System is using AC power.", LogSeverityInfo, "TestADTBattery")
	} else {
		logToSession("System is using battery power.", LogSeverityInfo, "TestADTBattery")
	}
	return info, nil
}

// TestADTNetworkConnection is the Go port of Test-ADTNetworkConnection: it
// reports whether any non-loopback network interface is up and has an
// address.
//
// Deviation from PSADT: the PowerShell function uses Get-NetAdapter to filter
// on physical adapters of specific interface types (Ethernet/Wi-Fi). Go's
// stdlib net.Interfaces cannot distinguish physical adapters or the WMI
// interface type without Windows-specific APIs, so this port reports any
// connected non-loopback interface, matching the intent (an active local
// network connection exists).
//
// An active ADT session is NOT required.
func TestADTNetworkConnection(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	logToSession("Checking if system has an active network connection...", LogSeverityInfo, "TestADTNetworkConnection")
	connected, err := anyNetworkConnection()
	if err != nil {
		return false, err
	}
	if connected {
		logToSession("Active network connection found.", LogSeverityInfo, "TestADTNetworkConnection")
	} else {
		logToSession("No active network connection found.", LogSeverityInfo, "TestADTNetworkConnection")
	}
	return connected, nil
}

// TestADTPowerPoint is the Go port of Test-ADTPowerPoint: it reports whether
// PowerPoint is presenting, either by a POWERPNT process owning a window whose
// title matches "^PowerPoint(-| Slide Show)", or by the shell reporting a
// presentation/busy notification state.
//
// An active ADT session is NOT required.
func TestADTPowerPoint(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	logToSession(
		"Checking if PowerPoint is in either fullscreen slideshow mode or presentation mode...",
		LogSeverityInfo,
		"TestADTPowerPoint",
	)
	return ssPowerPointActive()
}

// TestADTMicrophoneInUse is the Go port of Test-ADTMicrophoneInUse: it reports
// whether the microphone is currently in use.
//
// Deviation from PSADT: the PowerShell/C# implementation enumerates WASAPI
// audio sessions. This port reads the Capability Access Manager consent store
// (HKCU\...\ConsentStore\microphone\*) and treats a LastUsedTimeStop of 0 (an
// app that started but has not stopped using the microphone) as "in use". No
// WMI or COM is required.
//
// An active ADT session is NOT required.
func TestADTMicrophoneInUse(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	logToSession("Testing whether the device's microphone is in use...", LogSeverityInfo, "TestADTMicrophoneInUse")
	inUse, err := microphoneInUse(registryBackend())
	if err != nil {
		return false, err
	}
	if inUse {
		logToSession("The device's microphone is currently in use.", LogSeverityInfo, "TestADTMicrophoneInUse")
	} else {
		logToSession("The device's microphone is currently not in use.", LogSeverityInfo, "TestADTMicrophoneInUse")
	}
	return inUse, nil
}

// TestADTOobeCompleted is the Go port of Test-ADTOobeCompleted: it reports
// whether the device's Out-of-Box Experience has completed, via the kernel32
// OOBEComplete API.
//
// An active ADT session is NOT required.
func TestADTOobeCompleted(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	return ssOobeCompleted()
}

// TestADTEspActive is the Go port of Test-ADTEspActive: it reports whether the
// device is within an Autopilot Enrollment Status Page (ESP) phase. It checks
// for a running wwahost process, then that the OOBE has completed, then the
// per-user FirstSync enrollment state in the registry.
//
// Deviation from PSADT: the PowerShell function scopes the wwahost process and
// FirstSync registry lookup to the actively logged-on user's session/SID via
// the client/server plumbing. That plumbing is out of scope here, so this port
// checks any wwahost process and any HKLM\...\Enrollments\*\FirstSync\<SID>
// entry with an unset/zero IsSyncDone flag.
//
// An active ADT session is NOT required.
func TestADTEspActive(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	logToSession("Testing whether Windows is currently in a device or user ESP state.", LogSeverityInfo, "TestADTEspActive")

	running, err := ssWwahostRunning()
	if err != nil {
		return false, err
	}
	if !running {
		logToSession("Current ESP state is [false]. Reason: [There is no wwahost process currently running].", LogSeverityInfo, "TestADTEspActive")
		return false, nil
	}

	oobeDone, err := ssOobeCompleted()
	if err != nil {
		return false, err
	}
	if !oobeDone {
		logToSession("Current ESP state is [true]. Reason: [Device is still within the OOBE phase].", LogSeverityInfo, "TestADTEspActive")
		return true, nil
	}

	active, err := espRegistryActive(registryBackend())
	if err != nil {
		return false, err
	}
	logToSession(fmt.Sprintf("Current ESP state is [%t]. Reason: [Based on IsSyncDone flag within the registry].", active), LogSeverityInfo, "TestADTEspActive")
	return active, nil
}

// GetADTPendingReboot is the Go port of Get-ADTPendingReboot: it aggregates the
// classic pending-reboot registry signals (Component Based Servicing, Windows
// Update, App-V, Intune, and PendingFileRenameOperations) into a
// PendingRebootInfo.
//
// Deviation from PSADT: the SCCM/ConfigMgr client signal is queried via the
// root/ccm/ClientSDK WMI provider in PowerShell. WMI is unavailable in this
// build, so IsSCCMClientRebootPending is always false and a note is added to
// ErrorMsg. IsSystemRebootPending therefore reflects CBS, Windows Update and
// PendingFileRenameOperations only (matching PSADT's aggregation, minus SCCM).
//
// An active ADT session is NOT required.
func GetADTPendingReboot(ctx context.Context) (PendingRebootInfo, error) {
	if err := ctx.Err(); err != nil {
		return PendingRebootInfo{}, fmt.Errorf("adt: %w", err)
	}
	hostname, _ := os.Hostname()
	logToSession("Getting the pending reboot status on the local computer ["+hostname+"].", LogSeverityInfo, "GetADTPendingReboot")

	info, err := pendingRebootStatus(registryBackend())
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.ComputerName = hostname
	info.LastBootUpTime = ssBootTime()
	return info, nil
}

// GetADTUserToastNotificationMode is the Go port of
// Get-ADTUserToastNotificationMode.
//
// Deviation from PSADT: the PowerShell function reads the logged-on user's
// Windows.UI.Notifications.ToastNotificationMode via WinRT through the
// client/server process. Neither WinRT nor that plumbing is available here, so
// this port approximates the mode from the shell's user notification state
// (SHQueryUserNotificationState): QUIET_TIME maps to PriorityOnly, and the
// presentation/busy/fullscreen states map to AlarmsOnly, otherwise
// Unrestricted. The returned bool is false when the state cannot be queried.
//
// An active ADT session is NOT required.
func GetADTUserToastNotificationMode(ctx context.Context) (ToastNotificationMode, bool, error) {
	if err := ctx.Err(); err != nil {
		return ToastNotificationModeUnrestricted, false, fmt.Errorf("adt: %w", err)
	}
	logToSession("Querying the active user's toast notification mode...", LogSeverityInfo, "GetADTUserToastNotificationMode")
	mode, ok, err := ssToastNotificationMode()
	if err != nil {
		return ToastNotificationModeUnrestricted, false, err
	}
	if !ok {
		logToSession("Unable to query the user's toast notification mode.", LogSeverityInfo, "GetADTUserToastNotificationMode")
		return ToastNotificationModeUnrestricted, false, nil
	}
	logToSession("The user's toast notification mode is ["+mode.String()+"].", LogSeverityInfo, "GetADTUserToastNotificationMode")
	return mode, true, nil
}

// GetADTPresentationSettingsEnabledUsers is the Go port of
// Get-ADTPresentationSettingsEnabledUsers: it returns the loaded-profile users
// who have "I am currently giving a presentation" enabled (the
// HKCU\Software\Microsoft\MobilePC\AdaptableSettings\Activity registry value).
//
// Deviation from PSADT: this port only inspects the HKEY_USERS hives already
// loaded (the equivalent of -SkipUnloadedProfiles); it does not mount the
// NTUSER.DAT of logged-off users. NTAccount resolution is best-effort.
//
// This function corresponds to a deprecated PSADT cmdlet. An active ADT
// session is NOT required.
func GetADTPresentationSettingsEnabledUsers(ctx context.Context) ([]PresentationUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: %w", err)
	}
	logToSession("Checking whether any logged on users are in presentation mode...", LogSeverityInfo, "GetADTPresentationSettingsEnabledUsers")
	sids, err := presentationEnabledUsers(registryBackend())
	if err != nil {
		return nil, err
	}
	users := make([]PresentationUser, 0, len(sids))
	for _, sid := range sids {
		account, aerr := convertAccountOrSID(sid)
		if aerr != nil {
			account = ""
		}
		users = append(users, PresentationUser{SID: sid, NTAccount: account})
	}
	if len(users) == 0 {
		logToSession("There are no logged on users in presentation mode.", LogSeverityInfo, "GetADTPresentationSettingsEnabledUsers")
	}
	return users, nil
}

// anyNetworkConnection reports whether any non-loopback interface is up and
// carries at least one IP address.
func anyNetworkConnection() (bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, fmt.Errorf("adt: enumerating network interfaces: %w", err)
	}
	for i := range ifaces {
		iface := ifaces[i]
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP != nil && !ipnet.IP.IsUnspecified() {
				return true, nil
			}
		}
	}
	return false, nil
}

// Registry paths for the pending-reboot signals (rooted at HKLM).
const (
	rebootKeyCBServicing = `SOFTWARE\Microsoft\Windows\CurrentVersion\Component Based Servicing\RebootPending`
	rebootKeyWindowsUpd  = `SOFTWARE\Microsoft\Windows\CurrentVersion\WindowsUpdate\Auto Update\RebootRequired`
	// rebootKeyAppV mirrors PSADT's literal path, which includes a doubled
	// "Software" segment.
	rebootKeyAppV        = `SOFTWARE\Software\Microsoft\AppV\Client\PendingTasks`
	rebootKeyIntune      = `SOFTWARE\Microsoft\IntuneManagementExtension\RebootSettings\RebootFlag`
	rebootKeySessionMgr  = `SYSTEM\CurrentControlSet\Control\Session Manager`
	rebootValFileRenames = "PendingFileRenameOperations"
)

// pendingRebootStatus aggregates the registry-based pending-reboot signals
// over the given backend. It is fully portable so it can be unit-tested with
// regkey.Fake.
func pendingRebootStatus(backend regkey.Backend) (PendingRebootInfo, error) {
	var info PendingRebootInfo

	cbs, err := backend.KeyExists("HKLM", rebootKeyCBServicing)
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.IsCBServicingRebootPending = cbs

	wu, err := backend.KeyExists("HKLM", rebootKeyWindowsUpd)
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.IsWindowsUpdateRebootPending = wu

	appv, err := backend.KeyExists("HKLM", rebootKeyAppV)
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.IsAppVRebootPending = appv

	intune, err := backend.KeyExists("HKLM", rebootKeyIntune)
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.IsIntuneClientRebootPending = intune

	renames, hasRenames, err := pendingFileRenames(backend)
	if err != nil {
		return PendingRebootInfo{}, err
	}
	info.PendingFileRenameOperations = renames
	info.IsFileRenameRebootPending = hasRenames

	// SCCM/ConfigMgr requires the root/ccm/ClientSDK WMI provider, which is
	// unavailable in this build.
	info.IsSCCMClientRebootPending = false
	info.ErrorMsg = append(info.ErrorMsg, "IsSCCMClientRebootPending not evaluated: WMI is unavailable in this build.")

	// Match PSADT: the system pending flag excludes App-V and Intune.
	info.IsSystemRebootPending = info.IsCBServicingRebootPending ||
		info.IsWindowsUpdateRebootPending ||
		info.IsFileRenameRebootPending ||
		info.IsSCCMClientRebootPending

	if info.IsCBServicingRebootPending {
		info.Reasons = append(info.Reasons, "Component Based Servicing")
	}
	if info.IsWindowsUpdateRebootPending {
		info.Reasons = append(info.Reasons, "Windows Update")
	}
	if info.IsFileRenameRebootPending {
		info.Reasons = append(info.Reasons, "Pending File Rename Operations")
	}
	if info.IsAppVRebootPending {
		info.Reasons = append(info.Reasons, "App-V Pending Tasks")
	}
	if info.IsIntuneClientRebootPending {
		info.Reasons = append(info.Reasons, "Intune Management Extension")
	}
	return info, nil
}

// pendingFileRenames reads the PendingFileRenameOperations multi-string value,
// returning its non-empty entries and whether any were present.
func pendingFileRenames(backend regkey.Backend) ([]string, bool, error) {
	v, err := backend.GetValue("HKLM", rebootKeySessionMgr, rebootValFileRenames)
	if err != nil {
		if isNotFound(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	var entries []string
	switch data := v.Data.(type) {
	case []string:
		entries = data
	case string:
		if data != "" {
			entries = []string{data}
		}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e) != "" {
			out = append(out, e)
		}
	}
	return out, len(out) > 0, nil
}

// microphoneConsentKey is the Capability Access Manager consent store for the
// microphone (rooted at HKCU).
const microphoneConsentKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone`

// microphoneInUse reports whether any app's consent-store entry shows the
// microphone as actively in use (LastUsedTimeStop == 0). It walks the direct
// app subkeys and one level of nesting (the "NonPackaged" grouping).
func microphoneInUse(backend regkey.Backend) (bool, error) {
	return consentStoreInUse(backend, "HKCU", microphoneConsentKey, 2)
}

// consentStoreInUse recursively (bounded by depth) inspects consent-store
// subkeys for a LastUsedTimeStop of 0.
func consentStoreInUse(backend regkey.Backend, hive, path string, depth int) (bool, error) {
	subkeys, err := backend.EnumSubkeys(hive, path)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, sub := range subkeys {
		child := path + `\` + sub
		v, err := backend.GetValue(hive, child, "LastUsedTimeStop")
		if err == nil {
			if u, ok := toRegistryUint64(v.Data); ok && u == 0 {
				return true, nil
			}
		} else if !isNotFound(err) {
			return false, err
		}
		if depth > 1 {
			nested, err := consentStoreInUse(backend, hive, child, depth-1)
			if err != nil {
				return false, err
			}
			if nested {
				return true, nil
			}
		}
	}
	return false, nil
}

// enrollmentsKey is the MDM enrollments root (rooted at HKLM).
const enrollmentsKey = `SOFTWARE\Microsoft\Enrollments`

// espRegistryActive reports whether any enrollment has a FirstSync entry whose
// IsSyncDone flag is unset or zero (an in-progress ESP).
func espRegistryActive(backend regkey.Backend) (bool, error) {
	enrollments, err := backend.EnumSubkeys("HKLM", enrollmentsKey)
	if err != nil {
		if isNotFound(err) {
			return false, nil
		}
		return false, err
	}
	for _, enrollment := range enrollments {
		firstSync := enrollmentsKey + `\` + enrollment + `\FirstSync`
		sids, err := backend.EnumSubkeys("HKLM", firstSync)
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return false, err
		}
		for _, sid := range sids {
			v, err := backend.GetValue("HKLM", firstSync+`\`+sid, "IsSyncDone")
			if err != nil {
				if isNotFound(err) {
					return true, nil // no IsSyncDone flag => still syncing
				}
				return false, err
			}
			if u, ok := toRegistryUint64(v.Data); ok && u == 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// presentationActivityKey is the per-user mobility "currently giving a
// presentation" activity value path (relative to a HKU\<SID> root).
const presentationActivityKey = `Software\Microsoft\MobilePC\AdaptableSettings\Activity`

// presentationEnabledUsers returns the SIDs of loaded user hives with
// presentation mode enabled.
func presentationEnabledUsers(backend regkey.Backend) ([]string, error) {
	sids, err := backend.EnumSubkeys("HKU", "")
	if err != nil {
		if isNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(sids))
	for _, sid := range sids {
		if !strings.HasPrefix(sid, "S-1-5-21-") || strings.HasSuffix(sid, "_Classes") {
			continue
		}
		v, err := backend.GetValue("HKU", sid+`\`+presentationActivityKey, "Activity")
		if err != nil {
			if isNotFound(err) {
				continue
			}
			return nil, err
		}
		if u, ok := toRegistryUint64(v.Data); ok && u == 0 {
			continue // present but disabled
		}
		out = append(out, sid)
	}
	return out, nil
}

// toRegistryUint64 coerces a registry value's data to a uint64 for the numeric
// flag comparisons above.
func toRegistryUint64(data any) (uint64, bool) {
	switch v := data.(type) {
	case uint64:
		return v, true
	case uint32:
		return uint64(v), true
	case int64:
		return uint64(v), true //#nosec G115 -- flag comparison only; sign is irrelevant
	case int:
		return uint64(v), true //#nosec G115 -- flag comparison only; sign is irrelevant
	default:
		return 0, false
	}
}

// isNotFound reports whether err wraps winerr.ErrNotFound.
func isNotFound(err error) bool {
	return err != nil && errors.Is(err, winerr.ErrNotFound)
}
