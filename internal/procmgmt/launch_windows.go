package procmgmt

import (
	"context"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// WindowsLauncher launches processes in the caller's own logon session via
// os/exec, honoring CreateNoWindow and WindowStyle through the process
// STARTUPINFO. It is the default Launcher on Windows builds.
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
	//#nosec G204 -- launching caller-specified deployment executables is this package's purpose
	cmd := exec.CommandContext(ctx, opts.FilePath)
	cmd.Dir = opts.WorkingDirectory
	attr := &syscall.SysProcAttr{
		HideWindow: opts.CreateNoWindow || opts.WindowStyle == WindowHidden,
	}
	if opts.CreateNoWindow {
		attr.CreationFlags |= windows.CREATE_NO_WINDOW
	}
	if opts.ArgumentList != "" {
		// Preserve the caller's raw argument string exactly, PSADT-style:
		// quote only the executable path and append the arguments verbatim.
		attr.CmdLine = syscall.EscapeArg(opts.FilePath) + " " + opts.ArgumentList
	}
	cmd.SysProcAttr = attr
	return runCommand(ctx, cmd, opts)
}
