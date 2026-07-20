//go:build windows

package dialogclient

import (
	"context"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// MessageBoxTimeoutW is undocumented but stable since Windows XP (used by
// Windows itself and countless deployment tools): a MessageBoxW whose fifth
// parameter is a milliseconds timeout, returning mbTimedOut on expiry.
var (
	moduser32              = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxTimeoutW = moduser32.NewProc("MessageBoxTimeoutW")
)

// mbTimedOut is MessageBoxTimeoutW's timeout return value.
const mbTimedOut wm.MESSAGEBOX_RESULT = 32000

// messageBoxTimeout shows a MessageBox honoring an optional timeout (zero
// waits forever).
func messageBoxTimeout(text, caption string, style wm.MESSAGEBOX_STYLE, timeout time.Duration) (wm.MESSAGEBOX_RESULT, error) {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return 0, winerr.Wrap("dialogclient: encoding message text", winerr.ErrInvalidOption)
	}
	captionPtr, err := windows.UTF16PtrFromString(caption)
	if err != nil {
		return 0, winerr.Wrap("dialogclient: encoding caption", winerr.ErrInvalidOption)
	}
	ms := uintptr(windows.INFINITE)
	if timeout > 0 {
		ms = uintptr(timeout.Milliseconds())
	}
	ret, _, _ := procMessageBoxTimeoutW.Call(
		0,
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(unsafe.Pointer(captionPtr)),
		uintptr(style),
		0, // wLanguageId
		ms,
	)
	if ret == 0 {
		return 0, winerr.Wrap("dialogclient: MessageBoxTimeoutW", winerr.ErrDialogUnavailable)
	}
	return wm.MESSAGEBOX_RESULT(ret), nil //#nosec G115 -- MessageBox results are small ints
}

// renderModalNative is the WebView2-absent fallback: it draws the modal with a
// native MessageBox honoring Base.TimeoutSeconds (expiry returns
// ButtonTimeout, mapping to the toolkit's 1618 semantics). This remains a
// degraded path — MessageBox cannot host an input box, a selection list or a
// countdown. Input dialogs return the default value; list dialogs return the
// first item.
func renderModalNative(_ context.Context, p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error) {
	vm, err := BuildViewModel(p)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	text := nativeText(vm)
	style := nativeButtonStyle(len(vm.Buttons)) | nativeIconStyle(vm.Icon)
	style |= topMostStyle(p.Base.NotTopMost)

	got, err := messageBoxTimeout(text, p.Base.Title, style,
		time.Duration(p.Base.TimeoutSeconds)*time.Second)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	if got == mbTimedOut {
		// Explicit: the default button mapping would misattribute the
		// timeout result to a real button.
		return ipc.ModalDialogResult{Button: ButtonTimeout}, nil
	}

	res := ipc.ModalDialogResult{Button: mapNativeButton(got, vm.Buttons)}
	if vm.Input != nil {
		res.Input = vm.Input.DefaultValue
	}
	if vm.List != nil && len(vm.List.Items) > 0 {
		res.Selection = []string{vm.List.Items[0]}
	}
	return res, nil
}

// renderDialogBox draws the classic DialogBox with a native MessageBox,
// honoring the DialogBox timeout.
func renderDialogBox(p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error) {
	if p.Box == nil {
		return ipc.ModalDialogResult{}, winerr.Wrap(
			"dialogclient: DialogBox options missing",
			winerr.ErrInvalidOption,
		)
	}
	style := dialogBoxButtons(p.Box.Buttons) | nativeIconStyle(p.Box.Icon)
	style |= topMostStyle(p.Base.NotTopMost)
	got, err := messageBoxTimeout(p.Box.Text, p.Base.Title, style,
		time.Duration(p.Box.Timeout)*time.Second)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	if got == mbTimedOut {
		return ipc.ModalDialogResult{Button: ButtonTimeout}, nil
	}
	return ipc.ModalDialogResult{Button: mapDialogBoxResult(got)}, nil
}

// topMostStyle pins the MessageBox above normal windows unless opted out.
func topMostStyle(notTopMost bool) wm.MESSAGEBOX_STYLE {
	if notTopMost {
		return 0
	}
	return wm.MB_TOPMOST | wm.MB_SETFOREGROUND
}

// nativeText assembles the MessageBox body from the view model.
func nativeText(vm ViewModel) string {
	var b strings.Builder
	b.WriteString(vm.Message)
	if len(vm.Apps) > 0 {
		b.WriteString("\n")
		for _, a := range vm.Apps {
			b.WriteString("\n• ")
			if a.Description != "" {
				b.WriteString(a.Description)
			} else {
				b.WriteString(a.Name)
			}
		}
	}
	if vm.ShowDeferral && vm.DeferralsRemaining > 0 {
		b.WriteString("\n\nRemaining Deferrals: ")
		b.WriteString(strconv.Itoa(vm.DeferralsRemaining))
	}
	return b.String()
}

// nativeButtonStyle picks the MessageBox button set matching the button count.
func nativeButtonStyle(n int) wm.MESSAGEBOX_STYLE {
	switch {
	case n >= 3:
		return wm.MB_YESNOCANCEL
	case n == 2:
		return wm.MB_OKCANCEL
	default:
		return wm.MB_OK
	}
}

// nativeIconStyle maps a dialog icon name onto a MessageBox icon flag.
func nativeIconStyle(icon string) wm.MESSAGEBOX_STYLE {
	switch strings.ToLower(icon) {
	case "warning", "exclamation":
		return wm.MB_ICONWARNING
	case "error", "stop", "hand":
		return wm.MB_ICONERROR
	case "question":
		return wm.MB_ICONQUESTION
	case "information", "info", "asterisk":
		return wm.MB_ICONINFORMATION
	default:
		return 0
	}
}

// mapNativeButton maps a MessageBox result back to a view-model button id,
// pairing the affirmative result with the primary (last) button.
func mapNativeButton(got wm.MESSAGEBOX_RESULT, buttons []buttonVM) string {
	if len(buttons) == 0 {
		return ButtonTimeout
	}
	primary := buttons[len(buttons)-1].ID
	switch len(buttons) {
	case 1:
		return primary
	case 2:
		if got == wm.IDOK {
			return primary
		}
		return buttons[0].ID
	default:
		switch got {
		case wm.IDYES:
			return primary
		case wm.IDNO:
			return buttons[0].ID
		default:
			return buttons[1].ID
		}
	}
}

// dialogBoxButtons maps the DialogBox Buttons name onto a MessageBox flag.
func dialogBoxButtons(name string) wm.MESSAGEBOX_STYLE {
	switch strings.ToLower(name) {
	case "okcancel":
		return wm.MB_OKCANCEL
	case "yesno":
		return wm.MB_YESNO
	case "yesnocancel":
		return wm.MB_YESNOCANCEL
	case "retrycancel":
		return wm.MB_RETRYCANCEL
	case "abortretryignore":
		return wm.MB_ABORTRETRYIGNORE
	default:
		return wm.MB_OK
	}
}

// mapDialogBoxResult maps a MessageBox result onto the DialogBox button text.
func mapDialogBoxResult(got wm.MESSAGEBOX_RESULT) string {
	switch got {
	case wm.IDOK:
		return "OK"
	case wm.IDCANCEL:
		return "Cancel"
	case wm.IDYES:
		return "Yes"
	case wm.IDNO:
		return "No"
	case wm.IDABORT:
		return "Abort"
	case wm.IDRETRY:
		return "Retry"
	case wm.IDIGNORE:
		return "Ignore"
	default:
		return ButtonTimeout
	}
}
