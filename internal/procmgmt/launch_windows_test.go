package procmgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// cmdExe returns the absolute ComSpec path.
func cmdExe(t *testing.T) string {
	t.Helper()
	if cs := os.Getenv("ComSpec"); cs != "" {
		return cs
	}
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe")
}

func TestWindowsLauncherExitCodeAndStreams(t *testing.T) {
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:       cmdExe(t),
		ArgumentList:   `/c "echo out& exit 3"`,
		CreateNoWindow: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.ExitCode)
	assert.Contains(t, res.StdOut, "out")
}

func TestWindowsLauncherStandardInput(t *testing.T) {
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:       cmdExe(t),
		ArgumentList:   `/c findstr probe`,
		CreateNoWindow: true,
		StandardInput:  "stdin probe line\r\n",
	})
	require.NoError(t, err)
	assert.Contains(t, res.StdOut, "stdin probe line")
}

func TestWindowsLauncherPriorityClass(t *testing.T) {
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:       cmdExe(t),
		ArgumentList:   `/c exit 0`,
		CreateNoWindow: true,
		PriorityClass:  PriorityBelowNormal,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
}

func TestWindowsLauncherJobTracksChildren(t *testing.T) {
	// The root cmd spawns a detached child that outlives it; the job wait
	// must not return until the child is gone too.
	start := time.Now()
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:              cmdExe(t),
		ArgumentList:          `/c "start /b ping -n 3 127.0.0.1 >nul & exit 9"`,
		CreateNoWindow:        true,
		WaitForChildProcesses: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 9, res.ExitCode)
	assert.GreaterOrEqual(t, time.Since(start), time.Second,
		"WaitForChildProcesses should have waited for the detached ping child")
}

func TestWindowsLauncherKillChildrenWithParent(t *testing.T) {
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:                     cmdExe(t),
		ArgumentList:                 `/c "start /b ping -n 30 127.0.0.1 >nul & exit 2"`,
		CreateNoWindow:               true,
		KillChildProcessesWithParent: true,
	})
	require.NoError(t, err)
	assert.Equal(t, 2, res.ExitCode)
	// The 30-ping child is terminated when the job handle closes; nothing to
	// wait 30 seconds for. (Termination is asynchronous; presence is not
	// asserted to keep the test race-free.)
}

func TestWindowsLauncherTimeoutTerminates(t *testing.T) {
	_, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:       cmdExe(t),
		ArgumentList:   `/c ping -n 30 127.0.0.1 >nul`,
		CreateNoWindow: true,
		Timeout:        500 * time.Millisecond,
	})
	assert.ErrorIs(t, err, winerr.ErrTimeout)
}

func TestWindowsLauncherNoTerminateOnTimeout(t *testing.T) {
	start := time.Now()
	_, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:             cmdExe(t),
		ArgumentList:         `/c ping -n 10 127.0.0.1 >nul`,
		CreateNoWindow:       true,
		Timeout:              500 * time.Millisecond,
		NoTerminateOnTimeout: true,
	})
	assert.ErrorIs(t, err, winerr.ErrTimeout)
	assert.Less(t, time.Since(start), 5*time.Second,
		"the call must return at the timeout, not wait for the process")
}

func TestWindowsLauncherVerbOpen(t *testing.T) {
	res, err := WindowsLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:       cmdExe(t),
		ArgumentList:   `/c exit 4`,
		Verb:           "open",
		CreateNoWindow: true,
		WindowStyle:    WindowHidden,
	})
	require.NoError(t, err)
	assert.Equal(t, 4, res.ExitCode)
}
