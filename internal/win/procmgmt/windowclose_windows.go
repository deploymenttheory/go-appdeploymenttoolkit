package procmgmt

import (
	"context"
	"time"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// windowClosePollInterval matches PSADT's two-second re-check while waiting
// for prompted applications to close their windows.
const windowClosePollInterval = 2 * time.Second

// wmClose is the WM_CLOSE window message.
const wmClose = 0x0010

// CloseProcessWindowsGracefully ports the PromptToSave flow of the PSADT
// client (ClientExecutable.cs): every visible top-level window of the given
// processes is brought forward and sent WM_CLOSE — giving apps the chance to
// show their own save prompt — then the windows are polled until they are
// gone or the timeout elapses. Returns true when no windows remain.
//
// Must run in the same session as the target windows; window handles do not
// cross session boundaries.
func CloseProcessWindowsGracefully(ctx context.Context, pids []uint32, timeout time.Duration) (bool, error) {
	targets := make(map[uint32]bool, len(pids))
	for _, pid := range pids {
		targets[pid] = true
	}
	windowsOf := func() ([]WindowInfo, error) {
		all, err := WindowTitles()
		if err != nil {
			return nil, err
		}
		var mine []WindowInfo
		for _, w := range all {
			if targets[w.PID] {
				mine = append(mine, w)
			}
		}
		return mine, nil
	}

	open, err := windowsOf()
	if err != nil {
		return false, err
	}
	for _, w := range open {
		hwnd := foundation.HWND(w.Handle)
		_ = windowsandmessaging.SetForegroundWindow(hwnd)
		_ = windowsandmessaging.PostMessage(hwnd, wmClose, 0, 0)
	}

	deadline := time.Now().Add(timeout)
	for {
		open, err = windowsOf()
		if err != nil {
			return false, err
		}
		if len(open) == 0 {
			return true, nil
		}
		if time.Now().After(deadline) {
			return false, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(windowClosePollInterval):
		}
	}
}
