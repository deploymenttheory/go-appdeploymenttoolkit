package psadt

import (
	"context"
	"fmt"
	"path/filepath"
)

// EnableADTTerminalServerInstallMode is the Go port of
// Enable-ADTTerminalServerInstallMode: it switches a Remote Desktop Session
// Host into per-user install mode by running "change.exe User /Install".
//
// Deviation from PSADT: this port does not pre-check the current install mode
// (PSADT's TerminalServerUtilities.InAppInstallMode); change.exe is idempotent
// and reports success (exit code 1) when already in install mode.
func EnableADTTerminalServerInstallMode(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: EnableADTTerminalServerInstallMode: %w", err)
	}
	return invokeTerminalServerModeChange(ctx, "Install")
}

// DisableADTTerminalServerInstallMode is the Go port of
// Disable-ADTTerminalServerInstallMode: it switches a Remote Desktop Session
// Host back into per-user execute mode via "change.exe User /Execute".
func DisableADTTerminalServerInstallMode(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: DisableADTTerminalServerInstallMode: %w", err)
	}
	return invokeTerminalServerModeChange(ctx, "Execute")
}

// invokeTerminalServerModeChange ports Invoke-ADTTerminalServerModeChange: it
// runs change.exe with the requested mode. change.exe reports success with
// exit code 1, so that is the only success code accepted.
func invokeTerminalServerModeChange(ctx context.Context, mode string) error {
	logToSession(fmt.Sprintf("Changing terminal server into user %s mode.", terminalServerModeVerb(mode)),
		LogSeverityInfo, "InvokeADTTerminalServerModeChange")
	_, err := StartADTProcess(ctx, StartADTProcessOptions{
		FilePath:         changeExePath(),
		ArgumentList:     buildTerminalServerArgs(mode),
		CreateNoWindow:   true,
		SuccessExitCodes: []int{1},
	})
	return err
}

// buildTerminalServerArgs composes the change.exe argument string
// ("User /Install" or "User /Execute").
func buildTerminalServerArgs(mode string) string {
	return "User /" + mode
}

// terminalServerModeVerb renders the mode as the lower-case verb used in log
// messages ("install"/"execute").
func terminalServerModeVerb(mode string) string {
	if mode == "Install" {
		return "install"
	}
	return "execute"
}

// changeExePath returns %WINDIR%\System32\change.exe, matching PSADT's use of
// the system directory.
func changeExePath() string {
	if windir := windowsDir(); windir != "" {
		return filepath.Join(windir, "System32", "change.exe")
	}
	return "change.exe"
}
