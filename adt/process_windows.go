package adt

import (
	"context"
	"fmt"
	"strings"
	"time"

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
	s, _ := GetADTSession() // nil session means sessionless operation
	targets, err := resolveUserSessions(opts.UserName, opts.AllUsers)
	if err != nil {
		return nil, err
	}
	plan, err := buildProcessPlan(s, &opts.StartADTProcessOptions)
	if err != nil {
		return nil, err
	}
	var result *ProcessResult
	for _, target := range targets {
		processLog(fmt.Sprintf("Preparing to execute process [%s] for user [%s]...",
			plan.filePath, target.NTAccount()), LogSeverityInfo, "StartADTProcessAsUser")
		result, err = runProcessPlan(ctx, s, plan, opts.WaitForMsiExec, opts.MsiExecWaitTime,
			func(ctx context.Context, lo procmgmt.LaunchOptions) (*procmgmt.LaunchResult, error) {
				return procmgmt.LaunchAsUser(ctx, lo, target)
			})
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

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
