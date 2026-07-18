//go:build windows

package dialogclient

import (
	"context"
	"strconv"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	wm "github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// renderModalNative is the WebView2-absent fallback: it draws the modal with a
// native MessageBox. This is a degraded path — MessageBox cannot host an input
// box, a selection list or a countdown, and offers no dialog timeout (the
// MessageBoxTimeout API is not exposed by the bindings). Input dialogs return
// the default value; list dialogs return the first item; the ctx timeout and
// Base.TimeoutSeconds are not enforced here.
func renderModalNative(_ context.Context, p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error) {
	vm, err := BuildViewModel(p)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	text := nativeText(vm)
	style := nativeButtonStyle(len(vm.Buttons)) | nativeIconStyle(vm.Icon)

	got, err := wm.MessageBox(0, text, p.Base.Title, style)
	if err != nil {
		return ipc.ModalDialogResult{}, winerr.Wrap(
			"dialogclient: MessageBox",
			winerr.ErrDialogUnavailable,
		)
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

// renderDialogBox draws the classic DialogBox with a native MessageBox.
func renderDialogBox(p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error) {
	if p.Box == nil {
		return ipc.ModalDialogResult{}, winerr.Wrap(
			"dialogclient: DialogBox options missing",
			winerr.ErrInvalidOption,
		)
	}
	style := dialogBoxButtons(p.Box.Buttons) | nativeIconStyle(p.Box.Icon)
	got, err := wm.MessageBox(0, p.Box.Text, p.Base.Title, style)
	if err != nil {
		return ipc.ModalDialogResult{}, winerr.Wrap(
			"dialogclient: MessageBox",
			winerr.ErrDialogUnavailable,
		)
	}
	return ipc.ModalDialogResult{Button: mapDialogBoxResult(got)}, nil
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
