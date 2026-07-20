package procmgmt

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// WindowTitles ports the window sweep behind Get-ADTWindowTitle: it walks
// every visible top-level window (EnumWindows) and returns handle, title and
// owning process ID for each window carrying a non-empty title.
func WindowTitles() ([]WindowInfo, error) {
	var out []WindowInfo
	callback := syscall.NewCallback(func(hwnd uintptr, _ uintptr) uintptr {
		const continueEnumeration = 1
		hw := foundation.HWND(hwnd)
		if !windowsandmessaging.IsWindowVisible(hw) {
			return continueEnumeration
		}
		length, err := windowsandmessaging.GetWindowTextLength(hw)
		if err != nil || length <= 0 {
			return continueEnumeration // untitled windows are not interesting
		}
		buf := make([]uint16, length+1)
		copied, err := windowsandmessaging.GetWindowText(
			hw,
			foundation.PWSTR(unsafe.SliceData(buf)),
			length+1,
		)
		if err != nil || copied <= 0 {
			return continueEnumeration
		}
		var pid uint32
		windowsandmessaging.GetWindowThreadProcessId(hw, &pid)
		out = append(out, WindowInfo{
			Handle: hwnd,
			Title:  windows.UTF16ToString(buf[:copied]),
			PID:    pid,
		})
		return continueEnumeration
	})
	if err := windowsandmessaging.EnumWindows(
		windowsandmessaging.WNDENUMPROC(callback),
		0,
	); err != nil {
		return nil, fmt.Errorf("procmgmt: EnumWindows: %w", err)
	}
	return out, nil
}
