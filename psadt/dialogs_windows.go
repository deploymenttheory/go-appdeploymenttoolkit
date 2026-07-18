//go:build windows

package psadt

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogclient"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/shutdown"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
)

// newDialogServer resolves the interactive user session and returns a dialog
// server backed either by the in-process WebView renderer (when this process
// already runs in that session) or by a client re-execed into the session (when
// this process runs as SYSTEM or in a different session).
func newDialogServer(ctx context.Context, _ *DeploymentSession) (*dialogserver.DialogServer, error) {
	sessions, err := wts.NewNative().LoggedOnUsers()
	if err != nil {
		return nil, err
	}
	target, ok := activeUISession(sessions)
	if !ok {
		return nil, winerr.Wrap("psadt: no interactive user session", winerr.ErrDialogUnavailable)
	}
	if target.IsCurrent {
		return dialogserver.New(dialogclient.NewRenderer()), nil
	}
	return dialogserver.Launch(ctx, dialogserver.LaunchConfig{Session: target})
}

// activeUISession picks the session that should host the UI: the first active
// session, else the first session at all.
func activeUISession(sessions []wts.SessionInfo) (wts.SessionInfo, bool) {
	for _, s := range sessions {
		if s.IsActive {
			return s, true
		}
	}
	if len(sessions) > 0 {
		return sessions[0], true
	}
	return wts.SessionInfo{}, false
}

// initiateSystemRestart reboots the machine after delaySeconds, enabling the
// shutdown privilege first.
func initiateSystemRestart(_ context.Context, delaySeconds int, message string) error {
	if err := enableShutdownPrivilege(); err != nil {
		logToSession("Unable to enable shutdown privilege: "+err.Error(), LogSeverityWarning, "ShowADTInstallationRestartPrompt")
	}
	if delaySeconds < 0 {
		delaySeconds = 0
	}
	if err := shutdown.InitiateSystemShutdownEx(
		"",
		message,
		uint32(delaySeconds), //#nosec G115 -- delaySeconds is a small clamped non-negative value
		false,
		true,
		shutdown.SHUTDOWN_REASON(0),
	); err != nil {
		return winerr.Wrap("psadt: InitiateSystemShutdownEx", err)
	}
	return nil
}

// enableShutdownPrivilege grants SeShutdownPrivilege to the current process
// token, required by InitiateSystemShutdownEx.
func enableShutdownPrivilege() error {
	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY,
		&token,
	); err != nil {
		return fmt.Errorf("psadt: OpenProcessToken: %w", err)
	}
	defer func() { _ = token.Close() }()

	name, err := windows.UTF16PtrFromString("SeShutdownPrivilege")
	if err != nil {
		return fmt.Errorf("psadt: encoding privilege name: %w", err)
	}
	var luid windows.LUID
	if err := windows.LookupPrivilegeValue(nil, name, &luid); err != nil {
		return fmt.Errorf("psadt: LookupPrivilegeValue: %w", err)
	}
	tp := windows.Tokenprivileges{PrivilegeCount: 1}
	tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
	if err := windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil); err != nil {
		return fmt.Errorf("psadt: AdjustTokenPrivileges: %w", err)
	}
	return nil
}

// ifeoPath is the Image File Execution Options key under HKLM.
const ifeoPath = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`

// blockMarker tags the IFEO Debugger values this toolkit writes so Unblock can
// recognize (and only remove) its own hooks.
const blockMarker = "psadt-blocked"

// blockAppExecution installs the IFEO Debugger hooks and the cleanup task.
//
// Deviation/scope note: the Debugger command points at this executable tagged
// with blockMarker; rendering the "execution blocked" message from that launch
// is wired by the cmd/adt front end (Phase 4). The cleanup scheduled task is
// registered with schtasks.exe for simplicity (rather than the Task Scheduler
// COM API) and runs the toolkit's unblock path at next logon.
func blockAppExecution(ctx context.Context, s *DeploymentSession, names []string) error {
	if !s.IsAdmin() {
		return winerr.Wrap("psadt: blocking application execution requires administrator rights", winerr.ErrInvalidOption)
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("psadt: resolving executable: %w", err)
	}
	debugger := fmt.Sprintf(`"%s" %s`, exe, blockMarker)
	for _, name := range names {
		exeName := name
		if !strings.HasSuffix(strings.ToLower(exeName), ".exe") {
			exeName += ".exe"
		}
		key := fmt.Sprintf(`HKLM\%s\%s`, ifeoPath, exeName)
		if err := SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "Debugger", Value: debugger}); err != nil {
			return err
		}
		logToSession("Blocked execution of ["+exeName+"].", LogSeverityInfo, "BlockADTAppExecution")
	}
	if err := registerUnblockTask(exe, blockTaskName(s)); err != nil {
		logToSession("Failed to register unblock cleanup task: "+err.Error(), LogSeverityWarning, "BlockADTAppExecution")
	}
	return nil
}

// unblockAppExecution removes every IFEO Debugger hook this toolkit installed
// and deletes the cleanup task.
func unblockAppExecution(ctx context.Context, s *DeploymentSession) error {
	if !s.IsAdmin() {
		return winerr.Wrap("psadt: unblocking application execution requires administrator rights", winerr.ErrInvalidOption)
	}
	base, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoPath, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return fmt.Errorf("psadt: opening IFEO key: %w", err)
	}
	subs, err := base.ReadSubKeyNames(-1)
	_ = base.Close()
	if err != nil {
		return fmt.Errorf("psadt: enumerating IFEO subkeys: %w", err)
	}
	for _, sub := range subs {
		if !hasBlockMarker(sub) {
			continue
		}
		key := fmt.Sprintf(`HKLM\%s\%s`, ifeoPath, sub)
		_ = RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key, Name: "Debugger"})
		_ = RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key})
		logToSession("Unblocked execution of ["+sub+"].", LogSeverityInfo, "UnblockADTAppExecution")
	}
	if err := unregisterUnblockTask(blockTaskName(s)); err != nil {
		logToSession("Failed to remove unblock cleanup task: "+err.Error(), LogSeverityWarning, "UnblockADTAppExecution")
	}
	return nil
}

// hasBlockMarker reports whether the IFEO subkey's Debugger value was written by
// this toolkit.
func hasBlockMarker(subkey string) bool {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, ifeoPath+`\`+subkey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer func() { _ = k.Close() }()
	dbg, _, err := k.GetStringValue("Debugger")
	if err != nil {
		return false
	}
	return strings.Contains(dbg, blockMarker)
}

// blockTaskName derives the sanitized scheduled-task name for the session.
func blockTaskName(s *DeploymentSession) string {
	name := "PSAppDeployToolkit_" + s.InstallName() + "_BlockedApps"
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, name)
}

// registerUnblockTask creates the at-logon cleanup task via schtasks.exe.
func registerUnblockTask(exe, taskName string) error {
	tr := fmt.Sprintf(`"%s" unblock`, exe)
	//#nosec G204 -- fixed schtasks verb; exe is os.Executable and taskName is derived from install metadata
	cmd := exec.Command("schtasks", "/Create", "/TN", taskName, "/TR", tr,
		"/SC", "ONLOGON", "/RU", "SYSTEM", "/RL", "HIGHEST", "/F")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psadt: schtasks create: %w", err)
	}
	return nil
}

// unregisterUnblockTask deletes the cleanup task.
func unregisterUnblockTask(taskName string) error {
	//#nosec G204 -- fixed schtasks verb; taskName is derived from install metadata
	cmd := exec.Command("schtasks", "/Delete", "/TN", taskName, "/F")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("psadt: schtasks delete: %w", err)
	}
	return nil
}

// queryUserNotificationState reads the shell notification state.
func queryUserNotificationState() (int, error) {
	var state shell.QUERY_USER_NOTIFICATION_STATE
	if err := shell.SHQueryUserNotificationState(&state); err != nil {
		return 0, winerr.Wrap("psadt: SHQueryUserNotificationState", err)
	}
	return int(state), nil
}
