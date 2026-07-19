//go:build windows

package dialogclient

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogclient/assets"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/input/keyboardandmouse"
	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// Renderer is the Windows user-session UI renderer. Modal dialogs draw in a
// per-call WebView2 window (falling back to a native MessageBox when the
// runtime is absent); the progress dialog is a modeless WebView2 window that
// lives on its own OS thread and is refreshed with Eval; balloons use the shell
// notification icon.
type Renderer struct {
	mu       sync.Mutex
	progress *progressWindow
	tray     *trayIcon
}

// NewRenderer constructs the Windows renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// compile-time proof the renderer satisfies the server-side contract.
var _ dialogserver.Renderer = (*Renderer)(nil)

const (
	modalWidth  = 500
	modalHeight = 430
)

// ShowModal renders a modal dialog and blocks until it is answered, times out
// or is cancelled. DialogBox is drawn with a native MessageBox.
func (r *Renderer) ShowModal(
	ctx context.Context,
	p ipc.ModalDialogPayload,
) (ipc.ModalDialogResult, error) {
	if p.DialogType == ipc.DialogBox {
		return renderDialogBox(p)
	}
	return renderModalWebView(ctx, p)
}

// renderModalWebView draws the modal in a WebView2 window on a dedicated,
// OS-thread-locked goroutine. On WebView2 unavailability it falls back to the
// native renderer.
func renderModalWebView(
	ctx context.Context,
	p ipc.ModalDialogPayload,
) (ipc.ModalDialogResult, error) {
	vm, err := BuildViewModel(p)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	blob, err := vm.JSON()
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}

	type outcome struct {
		res      ipc.ModalDialogResult
		fellBack bool
	}
	done := make(chan outcome, 1)

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)

		w := webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  p.Base.Title,
				Width:  modalWidth,
				Height: modalHeight,
				Center: true,
			},
		})
		if w == nil {
			done <- outcome{fellBack: true}
			return
		}
		defer w.Destroy()

		var once sync.Once
		var result ipc.ModalDialogResult
		finish := func(res ipc.ModalDialogResult) {
			once.Do(func() {
				result = res
				w.Terminate()
			})
		}

		_ = w.Bind("__result", func(payload string) {
			var res ipc.ModalDialogResult
			_ = json.Unmarshal([]byte(payload), &res)
			finish(res)
		})
		w.Init("window.__DIALOG__ = " + blob + ";")
		w.SetSize(modalWidth, modalHeight, webview2.HintFixed)
		w.SetHtml(assets.DialogHTML())

		if p.Base.TimeoutSeconds > 0 {
			t := time.AfterFunc(time.Duration(p.Base.TimeoutSeconds)*time.Second, func() {
				finish(ipc.ModalDialogResult{Button: ButtonTimeout})
			})
			defer t.Stop()
		}
		stop := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				finish(ipc.ModalDialogResult{Button: ButtonCancel})
			case <-stop:
			}
		}()

		w.Run()
		close(stop)
		done <- outcome{res: result}
	}()

	o := <-done
	if o.fellBack {
		return renderModalNative(ctx, p)
	}
	return o.res, nil
}

// progressWindow owns the modeless progress WebView on its own OS thread.
type progressWindow struct {
	w    webview2.WebView
	done chan struct{}
}

// ShowProgress opens the progress window, or updates it if already open.
func (r *Renderer) ShowProgress(ctx context.Context, p ipc.ProgressPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.progress != nil {
		return r.updateProgressLocked(p)
	}
	view := BuildProgressView(p)
	blob, err := view.JSON()
	if err != nil {
		return err
	}

	pw := &progressWindow{done: make(chan struct{})}
	ready := make(chan bool, 1)
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		_ = windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED)

		w := webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  p.Base.Title,
				Width:  460,
				Height: 200,
				Center: true,
			},
		})
		if w == nil {
			ready <- false
			close(pw.done)
			return
		}
		defer w.Destroy()
		pw.w = w
		w.Init("window.__PROGRESS__ = " + blob + ";")
		w.SetSize(460, 200, webview2.HintFixed)
		w.SetHtml(assets.ProgressHTML())
		ready <- true
		w.Run()
		close(pw.done)
	}()

	if !<-ready {
		return winerr.Wrap("dialogclient: progress window unavailable", winerr.ErrDialogUnavailable)
	}
	r.progress = pw
	return nil
}

// UpdateProgress refreshes the open progress window; it is a no-op (open) when
// none is showing.
func (r *Renderer) UpdateProgress(_ context.Context, p ipc.ProgressPayload) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.progress == nil {
		return winerr.Wrap("dialogclient: no progress window open", winerr.ErrDialogUnavailable)
	}
	return r.updateProgressLocked(p)
}

// updateProgressLocked pushes a new view into the live window via Eval. The
// caller holds r.mu.
func (r *Renderer) updateProgressLocked(p ipc.ProgressPayload) error {
	view := BuildProgressView(p)
	blob, err := view.JSON()
	if err != nil {
		return err
	}
	w := r.progress.w
	w.Dispatch(func() { w.Eval("window.__setProgress(" + blob + ")") })
	return nil
}

// CloseProgress tears down the progress window.
func (r *Renderer) CloseProgress(_ context.Context) error {
	r.closeProgressWindow()
	return nil
}

// closeProgressWindow terminates and joins the progress goroutine. It takes no
// context so both CloseProgress and Close can share it without either creating
// a background context.
func (r *Renderer) closeProgressWindow() {
	r.mu.Lock()
	pw := r.progress
	r.progress = nil
	r.mu.Unlock()
	if pw == nil {
		return
	}
	pw.w.Terminate()
	<-pw.done
}

// MinimizeWindows minimizes every top-level window by asking the shell tray to
// run its "Minimize all" command.
func (r *Renderer) MinimizeWindows(_ context.Context) error {
	tray, err := wm.FindWindow("Shell_TrayWnd", "")
	if err != nil || tray == 0 {
		return winerr.Wrap("dialogclient: shell tray not found", winerr.ErrDialogUnavailable)
	}
	const minimizeAll = 419 // MIN_ALL
	if err := wm.PostMessage(tray, wm.WM_COMMAND, foundation.WPARAM(minimizeAll), 0); err != nil {
		return winerr.Wrap("dialogclient: MinimizeAll", winerr.ErrDialogUnavailable)
	}
	return nil
}

// GetWindowInfo returns the visible top-level windows matching p.WindowTitle
// (case-insensitive substring); an empty title returns every titled window.
func (r *Renderer) GetWindowInfo(
	_ context.Context,
	p ipc.SendKeysPayload,
) (ipc.WindowInfoResult, error) {
	windowsList, err := procmgmt.WindowTitles()
	if err != nil {
		return ipc.WindowInfoResult{}, err
	}
	out := ipc.WindowInfoResult{Windows: make([]ipc.WindowInfo, 0, len(windowsList))}
	needle := strings.ToLower(p.WindowTitle)
	for _, wnd := range windowsList {
		if needle != "" && !strings.Contains(strings.ToLower(wnd.Title), needle) {
			continue
		}
		out.Windows = append(out.Windows, ipc.WindowInfo{
			Handle: uint64(wnd.Handle),
			Title:  wnd.Title,
			PID:    wnd.PID,
		})
	}
	return out, nil
}

// SendKeys focuses the first window matching p.WindowTitle and types p.Keys as
// Unicode input.
//
// Deviation: the full SendKeys mini-language (e.g. {ENTER}, %^ modifiers) is
// not parsed; each rune is sent as a literal Unicode keystroke.
func (r *Renderer) SendKeys(ctx context.Context, p ipc.SendKeysPayload) error {
	info, err := r.GetWindowInfo(ctx, p)
	if err != nil {
		return err
	}
	if len(info.Windows) == 0 {
		return winerr.Wrap("dialogclient: no window matched "+p.WindowTitle, winerr.ErrNotFound)
	}
	target := foundation.HWND(uintptr(info.Windows[0].Handle))
	wm.SetForegroundWindow(target)
	return sendUnicode(p.Keys)
}

// RefreshDesktop refreshes the shell (icon associations) and broadcasts an
// environment-variable change, mirroring Update-ADTDesktop.
func (r *Renderer) RefreshDesktop(_ context.Context) error {
	shChangeNotifyAssoc()
	broadcastSettingChange("Environment")
	return nil
}

// Close releases any open progress window and tray icon.
func (r *Renderer) Close() {
	r.closeProgressWindow()
	r.mu.Lock()
	tray := r.tray
	r.tray = nil
	r.mu.Unlock()
	if tray != nil {
		tray.remove()
	}
}

// sendUnicode types s as a sequence of Unicode key down/up events.
func sendUnicode(s string) error {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}
	inputs := make([]keyboardandmouse.INPUT, 0, len(runes)*2)
	for _, ru := range runes {
		for _, up := range []bool{false, true} {
			in := keyboardandmouse.INPUT{Type: keyboardandmouse.INPUT_KEYBOARD}
			scan := uint16(
				ru,
			) //#nosec G115 -- BMP code unit; astral surrogates out of scope for SendKeys
			kb := keyboardandmouse.KEYBDINPUT{
				WScan:   scan,
				DwFlags: keyboardandmouse.KEYEVENTF_UNICODE,
			}
			if up {
				kb.DwFlags |= keyboardandmouse.KEYEVENTF_KEYUP
			}
			*(*keyboardandmouse.KEYBDINPUT)(unsafe.Pointer(&in.Anonymous)) = kb
			inputs = append(inputs, in)
		}
	}
	sent, err := keyboardandmouse.SendInput(inputs, int32(unsafe.Sizeof(keyboardandmouse.INPUT{})))
	if err != nil || sent == 0 {
		return winerr.Wrap("dialogclient: SendInput", winerr.ErrDialogUnavailable)
	}
	return nil
}

// broadcastSettingChange notifies top-level windows that a named setting area
// changed (WM_SETTINGCHANGE), best-effort.
func broadcastSettingChange(area string) {
	ptr := win32.UTF16Ptr(area)
	var result uintptr
	_, _ = wm.SendMessageTimeout(
		wm.HWND_BROADCAST,
		wm.WM_SETTINGCHANGE,
		0,
		foundation.LPARAM(uintptr(unsafe.Pointer(ptr))),
		wm.SMTO_ABORTIFHUNG,
		5000,
		&result,
	)
	runtime.KeepAlive(ptr)
}
