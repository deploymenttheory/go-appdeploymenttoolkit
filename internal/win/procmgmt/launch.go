// Package procmgmt implements the process-management primitives behind
// PSADT's Start-ADTProcess family: launching processes (optionally inside
// another user's logon session), probing named mutexes, enumerating running
// processes and window titles, inspecting PE architectures and retrying
// transient failures.
//
// Portable logic (option types, argument splitting, exit-code plumbing, PE
// parsing, retries) lives in the unsuffixed files so it is unit-testable on
// every platform; Windows syscall marshaling stays in *_windows.go files.
package procmgmt

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// WindowStyle mirrors System.Diagnostics.ProcessWindowStyle, the window
// disposition of Start-ADTProcess's -WindowStyle parameter.
type WindowStyle int

// WindowStyle values.
const (
	WindowNormal WindowStyle = iota
	WindowHidden
	WindowMinimized
	WindowMaximized
)

func (w WindowStyle) String() string {
	switch w {
	case WindowHidden:
		return "Hidden"
	case WindowMinimized:
		return "Minimized"
	case WindowMaximized:
		return "Maximized"
	default:
		return "Normal"
	}
}

// ParseWindowStyle parses the PSADT string form (case-insensitive). An empty
// string maps to Normal, matching Start-ADTProcess's default.
func ParseWindowStyle(s string) (WindowStyle, bool) {
	switch strings.ToLower(s) {
	case "", "normal":
		return WindowNormal, true
	case "hidden":
		return WindowHidden, true
	case "minimized":
		return WindowMinimized, true
	case "maximized":
		return WindowMaximized, true
	default:
		return WindowNormal, false
	}
}

// PriorityClass mirrors System.Diagnostics.ProcessPriorityClass, the
// scheduling priority of Start-ADTProcess's -PriorityClass parameter.
type PriorityClass int

// PriorityClass values.
const (
	PriorityNormal PriorityClass = iota
	PriorityIdle
	PriorityBelowNormal
	PriorityAboveNormal
	PriorityHigh
	PriorityRealTime
)

func (p PriorityClass) String() string {
	switch p {
	case PriorityIdle:
		return "Idle"
	case PriorityBelowNormal:
		return "BelowNormal"
	case PriorityAboveNormal:
		return "AboveNormal"
	case PriorityHigh:
		return "High"
	case PriorityRealTime:
		return "RealTime"
	default:
		return "Normal"
	}
}

// ParsePriorityClass parses the PSADT string form (case-insensitive). An
// empty string maps to Normal, matching Start-ADTProcess's default.
func ParsePriorityClass(s string) (PriorityClass, bool) {
	switch strings.ToLower(s) {
	case "", "normal":
		return PriorityNormal, true
	case "idle":
		return PriorityIdle, true
	case "belownormal":
		return PriorityBelowNormal, true
	case "abovenormal":
		return PriorityAboveNormal, true
	case "high":
		return PriorityHigh, true
	case "realtime":
		return PriorityRealTime, true
	default:
		return PriorityNormal, false
	}
}

// LaunchOptions carries the launch parameters shared by every launcher
// implementation. It is the Go analogue of PSADT's ProcessLaunchInfo.
type LaunchOptions struct {
	// FilePath is the executable to launch. Required.
	FilePath string
	// ArgumentList is the raw argument string appended to the command line.
	ArgumentList string
	// WorkingDirectory is the process working directory ("" inherits).
	WorkingDirectory string
	// WindowStyle controls the initial window disposition.
	WindowStyle WindowStyle
	// CreateNoWindow suppresses console window creation entirely.
	CreateNoWindow bool
	// UseShellExecute launches without stream redirection (StdOut/StdErr
	// will be empty), mirroring ProcessStartInfo.UseShellExecute.
	UseShellExecute bool
	// WaitForMsiExec asks the caller to gate the launch on the
	// Global\_MSIExecute mutex (honored by the adt facade, not here).
	WaitForMsiExec bool
	// MsiExecWaitTime bounds the WaitForMsiExec mutex wait.
	MsiExecWaitTime time.Duration
	// Timeout bounds the process runtime; zero waits indefinitely.
	Timeout time.Duration
	// StreamOutput requests StdOut/StdErr capture even when a console
	// window is shown.
	StreamOutput bool
	// NoWait launches the process without waiting for it to exit; the
	// returned result carries a zero exit code and empty streams.
	NoWait bool
	// PriorityClass is the scheduling priority (Windows launchers only).
	PriorityClass PriorityClass
	// StandardInput, when non-empty, is piped to the process's stdin.
	StandardInput string
	// NoTerminateOnTimeout leaves the process running when the wait times
	// out; the launch still returns winerr.ErrTimeout.
	NoTerminateOnTimeout bool
	// WaitForChildProcesses waits for the launched process's whole job
	// (children included) to finish, not just the root process (Windows
	// launchers only).
	WaitForChildProcesses bool
	// KillChildProcessesWithParent terminates any surviving child processes
	// once the root process exits (Windows launchers only).
	KillChildProcessesWithParent bool
	// Verb is a ShellExecuteEx verb (e.g. "runas"); when set the launch goes
	// through ShellExecuteEx with no stream capture (Windows launchers only).
	Verb string
	// DenyUserTermination denies PROCESS_TERMINATE on the launched process
	// to the interactive user (Windows launchers only).
	DenyUserTermination bool
	// OnStarted, when set, runs right after the process starts (before any
	// waiting); an error terminates the process and fails the launch. Used
	// by the Windows launcher for suspended-start job assignment.
	OnStarted func(pid int) error
}

// Validate checks the option set for launch-blocking mistakes.
func (o LaunchOptions) Validate() error {
	if o.FilePath == "" {
		return fmt.Errorf("procmgmt: FilePath is required: %w", winerr.ErrInvalidOption)
	}
	if o.WindowStyle < WindowNormal || o.WindowStyle > WindowMaximized {
		return fmt.Errorf(
			"procmgmt: WindowStyle %d out of range: %w",
			o.WindowStyle,
			winerr.ErrInvalidOption,
		)
	}
	if o.Timeout < 0 {
		return fmt.Errorf("procmgmt: negative Timeout: %w", winerr.ErrInvalidOption)
	}
	if o.PriorityClass < PriorityNormal || o.PriorityClass > PriorityRealTime {
		return fmt.Errorf(
			"procmgmt: PriorityClass %d out of range: %w",
			o.PriorityClass,
			winerr.ErrInvalidOption,
		)
	}
	return nil
}

// LaunchResult is the outcome of a completed (or detached) launch, the Go
// analogue of PSADT's ProcessResult.
type LaunchResult struct {
	ExitCode int
	StdOut   string
	StdErr   string
}

// Launcher is the process-launch seam. Implementations return a nil error
// for any exit code — classification against success/reboot lists is the
// caller's concern — and a non-nil error only when the process could not be
// launched or timed out.
type Launcher interface {
	Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error)
}

// ExecLauncher is the portable os/exec-based launcher used on non-Windows
// builds and in tests. ArgumentList is split on whitespace with double-quote
// grouping (see SplitArguments).
type ExecLauncher struct{}

// Launch implements Launcher.
func (ExecLauncher) Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := launchContext(ctx, opts)
	defer cancel()
	//#nosec G204 -- launching caller-specified deployment executables is this package's purpose
	cmd := exec.Command(opts.FilePath, SplitArguments(opts.ArgumentList)...)
	cmd.Dir = opts.WorkingDirectory
	return runCommand(ctx, cmd, opts)
}

// launchContext derives the timeout-bounded context for a launch.
func launchContext(ctx context.Context, opts LaunchOptions) (context.Context, context.CancelFunc) {
	if opts.Timeout > 0 {
		return context.WithTimeout(ctx, opts.Timeout)
	}
	return context.WithCancel(ctx)
}

// runCommand runs a prepared exec.Cmd honoring the shared LaunchOptions
// semantics: stream capture, stdin piping, NoWait detachment, the OnStarted
// hook, timeout classification (with optional NoTerminateOnTimeout) and
// exit-code extraction. Both the portable and the Windows launcher funnel
// through here. The cmd must NOT be built with exec.CommandContext:
// termination on cancellation is managed here so NoTerminateOnTimeout can
// leave the process running.
func runCommand(ctx context.Context, cmd *exec.Cmd, opts LaunchOptions) (*LaunchResult, error) {
	var stdout, stderr bytes.Buffer
	if !opts.UseShellExecute {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}
	if opts.StandardInput != "" {
		cmd.Stdin = strings.NewReader(opts.StandardInput)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("procmgmt: starting %q: %w", opts.FilePath, err)
	}
	if opts.OnStarted != nil {
		if err := opts.OnStarted(cmd.Process.Pid); err != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			return nil, fmt.Errorf("procmgmt: post-start setup for %q: %w", opts.FilePath, err)
		}
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	if opts.NoWait {
		// Detach: the goroutine above reaps the process so it never zombies.
		return &LaunchResult{}, nil
	}
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		if !opts.NoTerminateOnTimeout {
			_ = cmd.Process.Kill()
			<-done
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf(
				"procmgmt: process %q timed out: %w",
				opts.FilePath,
				winerr.ErrTimeout,
			)
		}
		return nil, fmt.Errorf("procmgmt: wait cancelled for %q: %w", opts.FilePath, ctx.Err())
	}
	result := &LaunchResult{StdOut: stdout.String(), StdErr: stderr.String()}
	if err == nil {
		return result, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return nil, fmt.Errorf("procmgmt: running %q: %w", opts.FilePath, err)
}

// SplitArguments splits a raw command-line argument string into an argument
// vector: fields are whitespace-separated, double quotes group fields, and a
// closing quote ends the group. Backslash escapes are passed through
// verbatim (Windows path friendliness).
func SplitArguments(argumentList string) []string {
	if argumentList == "" {
		return nil
	}
	args := make([]string, 0, 8)
	var cur strings.Builder
	inQuotes := false
	fieldOpen := false
	for _, r := range argumentList {
		switch {
		case r == '"':
			inQuotes = !inQuotes
			fieldOpen = true // "" is a legitimate empty argument
		case !inQuotes && (r == ' ' || r == '\t'):
			if fieldOpen {
				args = append(args, cur.String())
				cur.Reset()
				fieldOpen = false
			}
		default:
			cur.WriteRune(r)
			fieldOpen = true
		}
	}
	if fieldOpen {
		args = append(args, cur.String())
	}
	return args
}
