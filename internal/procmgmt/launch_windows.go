package procmgmt

import (
	"context"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
)

// WindowsLauncher launches processes in the caller's own logon session via
// os/exec, honoring CreateNoWindow, WindowStyle (through the process
// STARTUPINFO), PriorityClass, job-object child tracking and ShellExecuteEx
// verbs. It is the default Launcher on Windows builds.
//
// Limitation, kept deliberately thin (os/exec exposes HideWindow and
// CreationFlags but not STARTUPINFO.wShowWindow): Minimized/Maximized styles
// launch with a normal window; LaunchAsUser offers full show-window control.
type WindowsLauncher struct{}

// Launch implements Launcher.
func (WindowsLauncher) Launch(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := launchContext(ctx, opts)
	defer cancel()

	if opts.Verb != "" {
		return launchShellExecute(ctx, opts)
	}

	//#nosec G204 -- launching caller-specified deployment executables is this package's purpose
	cmd := exec.Command(opts.FilePath)
	cmd.Dir = opts.WorkingDirectory
	attr := &syscall.SysProcAttr{
		HideWindow: opts.CreateNoWindow || opts.WindowStyle == WindowHidden,
	}
	if opts.CreateNoWindow {
		attr.CreationFlags |= windows.CREATE_NO_WINDOW
	}
	attr.CreationFlags |= priorityCreationFlag(opts.PriorityClass)
	if opts.ArgumentList != "" {
		// Preserve the caller's raw argument string exactly, PSADT-style:
		// quote only the executable path and append the arguments verbatim.
		attr.CmdLine = syscall.EscapeArg(opts.FilePath) + " " + opts.ArgumentList
	}

	// Child-process tracking: start suspended, assign the process to a job
	// object, then resume — children can never escape the job (the reason
	// for the suspended start; assignment after a running start races).
	useJob := opts.WaitForChildProcesses || opts.KillChildProcessesWithParent
	var job *jobHandle
	if useJob {
		attr.CreationFlags |= windows.CREATE_SUSPENDED
		j, err := newJob(opts.KillChildProcessesWithParent)
		if err != nil {
			return nil, err
		}
		job = j
		defer func() { _ = job.Close() }()
	}
	callerOnStarted := opts.OnStarted
	opts.OnStarted = func(pid int) error {
		if job != nil {
			if err := job.assign(pid); err != nil {
				return err
			}
			if err := resumeProcessMainThreads(pid); err != nil {
				return err
			}
		}
		if opts.DenyUserTermination {
			if sid := InteractiveUserSID(); sid != nil {
				if err := DenyTerminateToUser(pid, sid); err != nil {
					return err
				}
			}
		}
		if callerOnStarted != nil {
			return callerOnStarted(pid)
		}
		return nil
	}
	cmd.SysProcAttr = attr

	res, err := runCommand(ctx, cmd, opts)
	if err != nil || opts.NoWait {
		return res, err
	}
	if opts.WaitForChildProcesses {
		if err := job.waitEmpty(ctx); err != nil {
			return res, err
		}
	}
	return res, nil
}

// priorityCreationFlag maps a PriorityClass onto CreateProcess creation flags.
func priorityCreationFlag(p PriorityClass) uint32 {
	switch p {
	case PriorityIdle:
		return windows.IDLE_PRIORITY_CLASS
	case PriorityBelowNormal:
		return windows.BELOW_NORMAL_PRIORITY_CLASS
	case PriorityAboveNormal:
		return windows.ABOVE_NORMAL_PRIORITY_CLASS
	case PriorityHigh:
		return windows.HIGH_PRIORITY_CLASS
	case PriorityRealTime:
		return windows.REALTIME_PRIORITY_CLASS
	default:
		return 0 // Normal: let CreateProcess default
	}
}

// launchShellExecute runs the target through ShellExecuteEx with the given
// verb (e.g. "runas" for an elevation prompt), porting Start-ADTProcess's
// -Verb parameter. SEE_MASK_NOCLOSEPROCESS keeps the process handle so the
// wait and exit-code semantics match the standard path; stream capture is
// unavailable, like -UseShellExecute.
func launchShellExecute(ctx context.Context, opts LaunchOptions) (*LaunchResult, error) {
	info := shell.SHELLEXECUTEINFOW{
		FMask:  shell.SEE_MASK_NOCLOSEPROCESS,
		LpVerb: win32.UTF16Ptr(opts.Verb),
		LpFile: win32.UTF16Ptr(opts.FilePath),
		NShow:  int32(showWindowCmd(opts)),
	}
	info.CbSize = uint32(unsafe.Sizeof(info))
	if opts.ArgumentList != "" {
		info.LpParameters = win32.UTF16Ptr(opts.ArgumentList)
	}
	if opts.WorkingDirectory != "" {
		info.LpDirectory = win32.UTF16Ptr(opts.WorkingDirectory)
	}
	if err := shell.ShellExecuteEx(&info); err != nil {
		return nil, fmt.Errorf("procmgmt: ShellExecuteEx(%q, verb %q): %w", opts.FilePath, opts.Verb, err)
	}
	if info.HProcess == 0 {
		// No waitable process (e.g. a DDE/document activation): nothing more
		// to report.
		return &LaunchResult{}, nil
	}
	h := windows.Handle(info.HProcess)
	defer func() { _ = windows.CloseHandle(h) }()
	if opts.NoWait {
		return &LaunchResult{}, nil
	}
	if err := waitProcessOpts(ctx, h, opts); err != nil {
		return nil, err
	}
	var exitCode uint32
	if err := windows.GetExitCodeProcess(h, &exitCode); err != nil {
		return nil, fmt.Errorf("procmgmt: GetExitCodeProcess: %w", err)
	}
	return &LaunchResult{ExitCode: int(int32(exitCode))}, nil
}
