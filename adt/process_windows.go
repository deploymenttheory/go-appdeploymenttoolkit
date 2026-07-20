package adt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
)

// processLauncher is the launcher behind StartADTProcess on Windows.
var processLauncher procmgmt.Launcher = procmgmt.WindowsLauncher{}

// mutexAvailable probes a named system mutex.
func mutexAvailable(name string, wait time.Duration) (bool, error) {
	return procmgmt.MutexAvailable(name, wait)
}

// runningProcesses enumerates running processes matching the specs.
func runningProcesses(specs []procmgmt.ProcessSpec) ([]RunningProcess, error) {
	return procmgmt.RunningProcesses(specs)
}

// windowTitles enumerates visible titled top-level windows.
func windowTitles() ([]WindowInfo, error) {
	return procmgmt.WindowTitles()
}

// startADTProcessAsUser resolves the target logon sessions via WTS and
// launches the process in each with procmgmt.LaunchAsUser.
func startADTProcessAsUser(
	ctx context.Context,
	opts StartADTProcessAsUserOptions,
) (*ProcessResult, error) {
	sel, err := resolveTokenSelection(opts)
	if err != nil {
		return nil, err
	}
	s, _ := GetADTSession() // nil session means sessionless operation
	targets, err := resolveUserSessions(opts.UserName, opts.AllUsers)
	if err != nil {
		return nil, err
	}
	plan, err := buildProcessPlan(s, &opts.StartADTProcessOptions)
	if err != nil {
		return nil, err
	}
	userOpts := procmgmt.AsUserOptions{
		TokenSelection:              sel,
		InheritEnvironmentVariables: opts.InheritEnvironmentVariables,
		DenyUserTermination:         opts.DenyUserTermination,
	}
	// The DACL edit is handled per-target inside LaunchAsUser; the generic
	// own-session path must not re-apply it.
	plan.launch.DenyUserTermination = false
	var result *ProcessResult
	for _, target := range targets {
		processLog(fmt.Sprintf("Preparing to execute process [%s] for user [%s]...",
			plan.filePath, target.NTAccount()), LogSeverityInfo, "StartADTProcessAsUser")
		result, err = runProcessPlan(ctx, s, plan, opts.WaitForMsiExec, opts.MsiExecWaitTime,
			func(ctx context.Context, lo procmgmt.LaunchOptions) (*procmgmt.LaunchResult, error) {
				return procmgmt.LaunchAsUser(ctx, lo, target, userOpts)
			})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

// resolveTokenSelection validates the mutually exclusive token switches and
// maps them onto a procmgmt.TokenSelection.
func resolveTokenSelection(opts StartADTProcessAsUserOptions) (procmgmt.TokenSelection, error) {
	set := 0
	sel := procmgmt.TokenDefault
	if opts.UseLinkedAdminToken {
		set++
		sel = procmgmt.TokenLinkedAdmin
	}
	if opts.UseHighestAvailableToken {
		set++
		sel = procmgmt.TokenHighestAvailable
	}
	if opts.UseUnelevatedToken {
		set++
		sel = procmgmt.TokenUnelevated
	}
	if set > 1 {
		return procmgmt.TokenDefault, fmt.Errorf(
			"adt: UseLinkedAdminToken, UseHighestAvailableToken and UseUnelevatedToken are mutually exclusive: %w",
			ErrInvalidOption,
		)
	}
	return sel, nil
}

// decodeOEM decodes a raw byte string of console-OEM-code-page text via
// MultiByteToWideChar(CP_OEMCP); undecodable input passes through.
func decodeOEM(s string) string {
	if s == "" {
		return s
	}
	src := []byte(s)
	n, err := windows.MultiByteToWideChar(cpOEMCP, 0, &src[0], int32(len(src)), nil, 0)
	if err != nil || n <= 0 {
		return s
	}
	buf := make([]uint16, n)
	if _, err := windows.MultiByteToWideChar(cpOEMCP, 0, &src[0], int32(len(src)), &buf[0], n); err != nil {
		return s
	}
	return windows.UTF16ToString(buf)
}

// cpOEMCP is the CP_OEMCP code-page identifier.
const cpOEMCP = 1

// resolveUserSessions picks the WTS sessions to launch into: every session
// for AllUsers, the named user's session for UserName, else the first
// logged-on session (active sessions sort first).
func resolveUserSessions(userName string, allUsers bool) ([]wts.SessionInfo, error) {
	sessions, err := wts.NewNative().LoggedOnUsers()
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("adt: no user is logged on: %w", ErrNotFound)
	}
	if allUsers {
		return sessions, nil
	}
	if userName == "" {
		return sessions[:1], nil
	}
	for _, sess := range sessions {
		if strings.EqualFold(sess.UserName, userName) || strings.EqualFold(sess.NTAccount(), userName) {
			return []wts.SessionInfo{sess}, nil
		}
	}
	return nil, fmt.Errorf("adt: no logged-on session for user %q: %w", userName, ErrNotFound)
}
