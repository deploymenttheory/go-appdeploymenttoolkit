//go:build windows

package dialogclient

import (
	"context"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// trayIcon owns the hidden message window and the shell notification icon used
// to surface balloon tips / toasts.
type trayIcon struct {
	hwnd foundation.HWND
	uid  uint32
}

// ShowBalloon shows a tray balloon (converted to a toast on Windows 10+). It is
// best-effort: modern Windows may suppress the tray icon glyph, but the toast
// still surfaces via the info flags.
func (r *Renderer) ShowBalloon(_ context.Context, p ipc.BalloonPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tray == nil {
		t, err := newTrayIcon()
		if err != nil {
			return err
		}
		r.tray = t
	}
	return r.tray.balloon(p)
}

// newTrayIcon creates the hidden message-only window and registers the base
// notification icon. The window class is registered against a NULL module
// handle, which the window manager accepts for an in-process class.
func newTrayIcon() (*trayIcon, error) {
	className := win32.UTF16Ptr("PSADTBalloonHost")
	proc := syscall.NewCallback(func(hwnd, msg, wparam, lparam uintptr) uintptr {
		return uintptr(wm.DefWindowProc(
			foundation.HWND(
				hwnd,
			),
			uint32(msg), //#nosec G115 -- window message ids always fit in uint32
			foundation.WPARAM(wparam),
			foundation.LPARAM(lparam),
		))
	})
	cls := wm.WNDCLASSW{
		LpfnWndProc:   wm.WNDPROC(proc),
		LpszClassName: foundation.PWSTR(className),
	}
	// A duplicate registration (class already exists) is harmless; ignore it.
	_, _ = wm.RegisterClass(&cls)

	hwnd, err := wm.CreateWindowEx(
		0,
		"PSADTBalloonHost",
		"PSADTBalloonHost",
		0,
		0, 0, 0, 0,
		wm.HWND_MESSAGE,
		0,
		0,
		nil,
	)
	if err != nil || hwnd == 0 {
		return nil, winerr.Wrap("dialogclient: CreateWindowEx", winerr.ErrDialogUnavailable)
	}

	t := &trayIcon{hwnd: hwnd, uid: 1}
	data := t.baseData(wm.WM_APP)
	if !shell.Shell_NotifyIcon(shell.NIM_ADD, &data) {
		// Fall back to sending the balloon straight through NIM_MODIFY.
		_ = wm.DestroyWindow(hwnd)
		return nil, winerr.Wrap(
			"dialogclient: Shell_NotifyIcon(NIM_ADD)",
			winerr.ErrDialogUnavailable,
		)
	}
	return t, nil
}

// baseData builds a NOTIFYICONDATAW carrying only the identity fields.
func (t *trayIcon) baseData(callbackMsg uint32) shell.NOTIFYICONDATAW {
	var d shell.NOTIFYICONDATAW
	d.CbSize = uint32(unsafe.Sizeof(d))
	d.HWnd = t.hwnd
	d.UID = t.uid
	d.UFlags = shell.NIF_MESSAGE
	d.UCallbackMessage = callbackMsg
	return d
}

// balloon updates the icon to display a balloon/toast with the given content.
func (t *trayIcon) balloon(p ipc.BalloonPayload) error {
	d := t.baseData(wm.WM_APP)
	d.UFlags |= shell.NIF_INFO
	copyUTF16(d.SzInfo[:], p.Text)
	copyUTF16(d.SzInfoTitle[:], p.Title)
	d.DwInfoFlags = balloonInfoFlag(p.Icon)
	if !shell.Shell_NotifyIcon(shell.NIM_MODIFY, &d) {
		return winerr.Wrap(
			"dialogclient: Shell_NotifyIcon(NIM_MODIFY)",
			winerr.ErrDialogUnavailable,
		)
	}
	return nil
}

// remove deletes the notification icon and destroys the host window.
func (t *trayIcon) remove() {
	d := t.baseData(wm.WM_APP)
	_ = shell.Shell_NotifyIcon(shell.NIM_DELETE, &d)
	_ = wm.DestroyWindow(t.hwnd)
}

// balloonInfoFlag maps the balloon icon name onto the shell info-tip flag.
func balloonInfoFlag(icon string) shell.NOTIFY_ICON_INFOTIP_FLAGS {
	switch strings.ToLower(icon) {
	case "warning":
		return shell.NIIF_WARNING
	case "error":
		return shell.NIIF_ERROR
	case "none":
		return shell.NIIF_NONE
	default:
		return shell.NIIF_INFO
	}
}

// copyUTF16 writes s (UTF-16, NUL-terminated) into a fixed-size array, never
// overrunning it.
func copyUTF16(dst []uint16, s string) {
	enc := windows.StringToUTF16(s)
	if len(enc) > len(dst) {
		enc = enc[:len(dst)]
		enc[len(enc)-1] = 0
	}
	copy(dst, enc)
}

// shChangeNotifyAssoc tells the shell that file associations changed, prompting
// a desktop/icon refresh (the SHChangeNotify half of Update-ADTDesktop).
func shChangeNotifyAssoc() {
	shell.SHChangeNotify(int32(shell.SHCNE_ASSOCCHANGED), shell.SHCNF_IDLIST, nil, nil)
}
