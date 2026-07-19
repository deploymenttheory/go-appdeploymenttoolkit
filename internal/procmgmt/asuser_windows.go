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

// LaunchAsUser ports the SYSTEM-to-user-session launch behind
// Start-ADTProcessAsUser: it obtains the session user's primary token
// (WTSQueryUserToken, SYSTEM-only), duplicates it, builds the user's
// environment block and calls CreateProcessAsUser on the interactive
// desktop (winsta0\default), then waits for exit honoring ctx and
// opts.Timeout. StdOut/StdErr capture is not available across the session
// boundary; the result carries the exit code only.
//
// This is a thin marshaling layer over the Win32 calls: it cannot be
// exercised by tests in this repository and must stay obviously correct by
// inspection.
func LaunchAsUser(
	ctx context.Context,
	opts LaunchOptions,
	session wts.SessionInfo,
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

	// 2. Duplicate into a primary token usable by CreateProcessAsUser.
	var primaryToken windows.Token
	if err := windows.DuplicateTokenEx(
		windows.Token(userToken),
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primaryToken,
	); err != nil {
		return nil, fmt.Errorf("procmgmt: DuplicateTokenEx: %w", err)
	}
	defer func() { _ = primaryToken.Close() }()

	// 3. The user's environment block (their HKCU-derived variables).
	var envBlock unsafe.Pointer
	if err := environment.CreateEnvironmentBlock(
		&envBlock,
		foundation.HANDLE(primaryToken),
		false,
	); err != nil {
		return nil, fmt.Errorf("procmgmt: CreateEnvironmentBlock: %w", err)
	}
	defer func() { _ = environment.DestroyEnvironmentBlock(envBlock) }()

	// 4. CreateProcessAsUser on the interactive desktop. The command line
	// buffer must be mutable (CreateProcessW writes into it).
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
	_ = windows.CloseHandle(windows.Handle(pi.HThread)) // thread handle is never used

	if opts.NoWait {
		return &LaunchResult{}, nil
	}

	// 5. Wait for exit in short slices so ctx cancellation and Timeout are
	// both honored, then collect the exit code.
	if err := waitProcess(ctx, windows.Handle(pi.HProcess), opts.Timeout); err != nil {
		return nil, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(windows.Handle(pi.HProcess), &exitCode); err != nil {
		return nil, fmt.Errorf("procmgmt: GetExitCodeProcess: %w", err)
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
// On timeout or cancellation the process is terminated (Start-ADTProcess
// default; -NoTerminateOnTimeout is not ported).
func waitProcess(ctx context.Context, h windows.Handle, timeout time.Duration) error {
	var deadline time.Time
	if timeout > 0 {
		deadline = time.Now().Add(timeout)
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = windows.TerminateProcess(h, uint32(0xFFFFFFFF))
			return fmt.Errorf("procmgmt: wait cancelled: %w", err)
		}
		slice := asUserWaitSlice
		if !deadline.IsZero() {
			remaining := time.Until(deadline)
			if remaining <= 0 {
				_ = windows.TerminateProcess(h, uint32(0xFFFFFFFF))
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
