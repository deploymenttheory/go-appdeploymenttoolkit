package procmgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/environment"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/remotedesktop"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// asUserWaitSlice is the polling slice used while waiting for the child
// process so context cancellation is honored promptly.
const asUserWaitSlice = 250 * time.Millisecond

// AsUserOptions carries the target-user behaviors specific to LaunchAsUser.
type AsUserOptions struct {
	// TokenSelection picks the token variant (default/linked-admin/highest/
	// unelevated) the process runs under.
	TokenSelection TokenSelection
	// InheritEnvironmentVariables passes the calling process's environment
	// instead of the target user's HKCU-derived block.
	InheritEnvironmentVariables bool
	// DenyUserTermination denies PROCESS_TERMINATE on the child to the
	// session user, so they cannot kill the deployment-launched process.
	DenyUserTermination bool
}

// LaunchAsUser ports the SYSTEM-to-user-session launch behind
// Start-ADTProcessAsUser: it obtains the session user's primary token
// (WTSQueryUserToken, SYSTEM-only), applies the requested token selection,
// builds the environment block and calls CreateProcessAsUser on the
// interactive desktop (winsta0\default), then waits for exit honoring ctx
// and opts.Timeout. StdOut/StdErr capture is not available across the
// session boundary; the result carries the exit code only.
//
// This is a thin marshaling layer over the Win32 calls: it cannot be
// exercised by tests in this repository and must stay obviously correct by
// inspection.
func LaunchAsUser(
	ctx context.Context,
	opts LaunchOptions,
	session wts.SessionInfo,
	userOpts AsUserOptions,
) (*LaunchResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("procmgmt: LaunchAsUser: %w", err)
	}

	// 1. Session token: only obtainable when running as LocalSystem with
	// SE_TCB_NAME, exactly like PSADT's ProcessManager.
	var userToken foundation.HANDLE
	if err := remotedesktop.WTSQueryUserToken(session.SessionID, &userToken); err != nil {
		return nil, fmt.Errorf(
			"procmgmt: WTSQueryUserToken(session %d): %w",
			session.SessionID,
			err,
		)
	}
	defer func() { _ = windows.CloseHandle(windows.Handle(userToken)) }()

	// 2. Select and duplicate into a primary token usable by
	// CreateProcessAsUser (base, linked-admin, highest-available or
	// unelevated per the caller's choice).
	primaryToken, err := selectUserToken(windows.Token(userToken), userOpts.TokenSelection)
	if err != nil {
		return nil, err
	}
	defer func() { _ = primaryToken.Close() }()

	// 3. The environment block: the user's own HKCU-derived variables, or
	// the calling process's environment when inheritance is requested (a
	// nil block means "inherit parent" to CreateProcessAsUser).
	var envBlock unsafe.Pointer
	if !userOpts.InheritEnvironmentVariables {
		if err := environment.CreateEnvironmentBlock(
			&envBlock,
			foundation.HANDLE(primaryToken),
			false,
		); err != nil {
			return nil, fmt.Errorf("procmgmt: CreateEnvironmentBlock: %w", err)
		}
		defer func() { _ = environment.DestroyEnvironmentBlock(envBlock) }()
	}

	// 4. CreateProcessAsUser on the interactive desktop. The command line
	// buffer must be mutable (CreateProcessW writes into it). Job tracking
	// starts the process suspended so children can never escape the job.
	cmdLine := windows.EscapeArg(opts.FilePath)
	if opts.ArgumentList != "" {
		cmdLine += " " + opts.ArgumentList
	}
	cmdLineBuf, err := windows.UTF16FromString(cmdLine)
	if err != nil {
		return nil, fmt.Errorf("procmgmt: encoding command line: %w", err)
	}

	si := threading.STARTUPINFOW{
		LpDesktop:   foundation.PWSTR(win32.UTF16Ptr(`winsta0\default`)),
		DwFlags:     threading.STARTF_USESHOWWINDOW,
		WShowWindow: showWindowCmd(opts),
	}
	si.Cb = uint32(unsafe.Sizeof(si))

	flags := threading.CREATE_UNICODE_ENVIRONMENT
	if opts.CreateNoWindow {
		flags |= threading.CREATE_NO_WINDOW
	}
	flags |= threading.PROCESS_CREATION_FLAGS(priorityCreationFlag(opts.PriorityClass))
	useJob := opts.WaitForChildProcesses || opts.KillChildProcessesWithParent
	var job *jobHandle
	if useJob {
		flags |= threading.CREATE_SUSPENDED
		if job, err = newJob(opts.KillChildProcessesWithParent); err != nil {
			return nil, err
		}
		defer func() { _ = job.Close() }()
	}

	var pi threading.PROCESS_INFORMATION
	if err := threading.CreateProcessAsUser(
		foundation.HANDLE(primaryToken),
		opts.FilePath, // lpApplicationName: binding marshals string -> LPCWSTR
		foundation.PWSTR(unsafe.SliceData(cmdLineBuf)),
		nil,   // process attributes
		nil,   // thread attributes
		false, // no handle inheritance across the session boundary
		flags,
		envBlock,
		asUserWorkingDirectory(opts), // binding cannot pass NULL; always concrete
		&si,
		&pi,
	); err != nil {
		return nil, fmt.Errorf("procmgmt: CreateProcessAsUser(%q): %w", opts.FilePath, err)
	}
	defer func() { _ = windows.CloseHandle(windows.Handle(pi.HProcess)) }()

	// Post-start setup while (possibly) suspended: job assignment and the
	// deny-termination DACL, then resume.
	if job != nil {
		if err := job.assign(int(pi.DwProcessId)); err != nil {
			_ = windows.TerminateProcess(windows.Handle(pi.HProcess), uint32(0xFFFFFFFF))
			_ = windows.CloseHandle(windows.Handle(pi.HThread))
			return nil, err
		}
	}
	if userOpts.DenyUserTermination {
		if sid, _, _, err := windows.LookupSID("", session.NTAccount()); err == nil {
			if err := DenyTerminateToUser(int(pi.DwProcessId), sid); err != nil {
				_ = windows.TerminateProcess(windows.Handle(pi.HProcess), uint32(0xFFFFFFFF))
				_ = windows.CloseHandle(windows.Handle(pi.HThread))
				return nil, err
			}
		}
	}
	if useJob {
		if _, err := windows.ResumeThread(windows.Handle(pi.HThread)); err != nil {
			_ = windows.TerminateProcess(windows.Handle(pi.HProcess), uint32(0xFFFFFFFF))
			_ = windows.CloseHandle(windows.Handle(pi.HThread))
			return nil, fmt.Errorf("procmgmt: ResumeThread: %w", err)
		}
	}
	_ = windows.CloseHandle(windows.Handle(pi.HThread))

	if opts.NoWait {
		return &LaunchResult{}, nil
	}

	// 5. Wait for exit in short slices so ctx cancellation and Timeout are
	// both honored, then collect the exit code and drain the job.
	if err := waitProcess(ctx, windows.Handle(pi.HProcess), opts.Timeout, opts.NoTerminateOnTimeout); err != nil {
		return nil, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(windows.Handle(pi.HProcess), &exitCode); err != nil {
		return nil, fmt.Errorf("procmgmt: GetExitCodeProcess: %w", err)
	}
	if opts.WaitForChildProcesses {
		if err := job.waitEmpty(ctx); err != nil {
			return nil, err
		}
	}
	return &LaunchResult{ExitCode: int(int32(exitCode))}, nil
}

// showWindowCmd maps LaunchOptions onto a STARTUPINFO wShowWindow value.
func showWindowCmd(opts LaunchOptions) uint16 {
	if opts.CreateNoWindow || opts.WindowStyle == WindowHidden {
		return uint16(windows.SW_HIDE)
	}
	switch opts.WindowStyle {
	case WindowMinimized:
		return uint16(windows.SW_SHOWMINIMIZED)
	case WindowMaximized:
		return uint16(windows.SW_SHOWMAXIMIZED)
	default:
		return uint16(windows.SW_SHOWNORMAL)
	}
}

// asUserWorkingDirectory picks the lpCurrentDirectory value. The binding
// marshals Go strings with UTF16Ptr, so an empty string becomes a pointer to
// "" rather than NULL — which CreateProcessAsUser rejects. Fall back to the
// executable's directory, then the process working directory.
func asUserWorkingDirectory(opts LaunchOptions) string {
	if opts.WorkingDirectory != "" {
		return opts.WorkingDirectory
	}
	if filepath.IsAbs(opts.FilePath) {
		return filepath.Dir(opts.FilePath)
	}
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return `C:\`
}

// waitProcess waits for the process handle to signal, polling in slices so
// ctx cancellation interrupts the wait. A zero timeout waits indefinitely.
// On timeout or cancellation the process is terminated unless
// noTerminateOnTimeout is set (Start-ADTProcess -NoTerminateOnTimeout), in
// which case it is left running and the timeout error still returns.
func waitProcess(ctx context.Context, h windows.Handle, timeout time.Duration, noTerminateOnTimeout bool) error {
	terminate := func() {
		if !noTerminateOnTimeout {
			_ = windows.TerminateProcess(h, uint32(0xFFFFFFFF))
		}
	}
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if err := ctx.Err(); err != nil {
			terminate()
			return fmt.Errorf("procmgmt: wait cancelled: %w", err)
		}
		slice := asUserWaitSlice
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				terminate()
				return fmt.Errorf(
					"procmgmt: process wait exceeded %s: %w",
					timeout,
					winerr.ErrTimeout,
				)
			}
			if remaining < slice {
				slice = remaining
			}
		}
		event, err := windows.WaitForSingleObject(
			h,
			uint32(slice.Milliseconds()),
		) //#nosec G115 -- slice is <= 250ms
		if err != nil {
			return fmt.Errorf("procmgmt: WaitForSingleObject: %w", err)
		}
		if event == windows.WAIT_OBJECT_0 {
			return nil
		}
	}
}

// waitProcessOpts is waitProcess parameterized from LaunchOptions.
func waitProcessOpts(ctx context.Context, h windows.Handle, opts LaunchOptions) error {
	return waitProcess(ctx, h, opts.Timeout, opts.NoTerminateOnTimeout)
}
