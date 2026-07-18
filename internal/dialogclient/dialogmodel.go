// Package dialogclient renders the toolkit's user-session dialogs. The primary
// path draws Fluent-style dialogs with an embedded WebView2 window; when the
// WebView2 runtime is absent it falls back to native TaskDialog/MessageBox
// controls. It also hosts the `client` subcommand entrypoint (ClientMain) that
// the deployment side re-execs into the interactive session and drives over
// the ipc protocol.
//
// This file holds the portable payload->viewmodel mapping: it turns an
// ipc.ModalDialogPayload into the JSON blob injected into the HTML bootstrap
// (window.__DIALOG__) and validates that the requested dialog is renderable.
// It contains no Windows-specific code and is unit-tested on every platform.
package dialogclient

import (
	"encoding/json"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Button identifiers echoed back in ipc.ModalDialogResult.Button. They are
// stable strings shared between the JS bootstrap and the Go result mapping.
const (
	ButtonContinue     = "Continue"
	ButtonDefer        = "Defer"
	ButtonClose        = "Close"
	ButtonLeft         = "Left"
	ButtonMiddle       = "Middle"
	ButtonRight        = "Right"
	ButtonRestartNow   = "RestartNow"
	ButtonRestartLater = "RestartLater"
	ButtonTimeout      = "Timeout"
	ButtonCancel       = "Cancel"
)

// buttonVM is one button in the rendered button row.
type buttonVM struct {
	ID   string `json:"id"`            // echoed as ModalDialogResult.Button
	Text string `json:"text"`          // display label
	Kind string `json:"kind"`          // "primary" | "secondary"
	Tip  string `json:"tip,omitempty"` // optional tooltip
}

// appVM is one running application shown in the close-apps list.
type appVM struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// inputVM configures the single-line input control.
type inputVM struct {
	DefaultValue string `json:"defaultValue"`
	Placeholder  string `json:"placeholder,omitempty"`
}

// listVM configures the selection list control.
type listVM struct {
	Items       []string `json:"items"`
	MultiSelect bool     `json:"multiSelect"`
}

// ViewModel is the JSON contract injected as window.__DIALOG__ and consumed by
// the JS bootstrap. It is a flattened, render-ready projection of the ipc
// payload: the Go side has already selected strings, buttons and the countdown
// behaviour so the JS only lays out and reports the pressed button id.
type ViewModel struct {
	Type         string `json:"type"` // ipc.DialogType value
	Title        string `json:"title"`
	Subtitle     string `json:"subtitle,omitempty"`
	Message      string `json:"message,omitempty"`
	MessageAlign string `json:"messageAlign,omitempty"`
	Icon         string `json:"icon,omitempty"`
	Fluent       bool   `json:"fluent"`
	AccentColor  string `json:"accentColor,omitempty"`
	LogoImage    string `json:"logoImage,omitempty"`
	BannerImage  string `json:"bannerImage,omitempty"`
	AppIconImage string `json:"appIconImage,omitempty"`

	Apps    []appVM    `json:"apps,omitempty"`
	Buttons []buttonVM `json:"buttons"`
	Input   *inputVM   `json:"input,omitempty"`
	List    *listVM    `json:"list,omitempty"`

	// CountdownSeconds, when > 0, fires CountdownButton on expiry.
	CountdownSeconds int    `json:"countdownSeconds,omitempty"`
	CountdownButton  string `json:"countdownButton,omitempty"`
	CountdownLabel   string `json:"countdownLabel,omitempty"`

	// Deferral chrome (close-apps dialog only).
	ShowDeferral       bool   `json:"showDeferral"`
	DeferralsRemaining int    `json:"deferralsRemaining,omitempty"`
	DeferralDeadline   string `json:"deferralDeadline,omitempty"`

	// TimeoutSeconds is the hard dialog timeout; the JS shows nothing for it
	// but the Go side arms a Terminate on expiry returning ButtonTimeout.
	TimeoutSeconds int `json:"timeoutSeconds,omitempty"`

	// Labels carries localized chrome strings (deferral captions, etc.).
	Labels map[string]string `json:"labels,omitempty"`
}

// JSON renders the view model as the string injected into window.__DIALOG__.
func (v ViewModel) JSON() (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", winerr.Wrap("dialogclient: marshaling view model", winerr.ErrInvalidOption)
	}
	return string(body), nil
}

// BuildViewModel projects an ipc.ModalDialogPayload onto a render-ready
// ViewModel, validating that the payload carries the options its dialog type
// requires. DialogBox is intentionally rejected: it is rendered with a native
// MessageBox, never through the WebView bootstrap.
func BuildViewModel(p ipc.ModalDialogPayload) (ViewModel, error) {
	vm := ViewModel{
		Type:           string(p.DialogType),
		Title:          p.Base.Title,
		Subtitle:       p.Base.Subtitle,
		Fluent:         p.Base.FluentStyle,
		AccentColor:    p.Base.AccentColor,
		LogoImage:      p.Base.LogoImage,
		BannerImage:    p.Base.BannerImage,
		AppIconImage:   p.Base.AppIconImage,
		TimeoutSeconds: p.Base.TimeoutSeconds,
	}
	if p.Base.Title == "" {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: dialog title is required",
			winerr.ErrInvalidOption,
		)
	}
	switch p.DialogType {
	case ipc.DialogCloseApps:
		return buildCloseApps(vm, p.CloseApps)
	case ipc.DialogCustom:
		return buildCustom(vm, p.Custom)
	case ipc.DialogInput:
		return buildInput(vm, p.Input)
	case ipc.DialogListSelection:
		return buildList(vm, p.List)
	case ipc.DialogRestart:
		return buildRestart(vm, p.Restart)
	case ipc.DialogBox:
		return ViewModel{}, winerr.Wrap(
			"dialogclient: DialogBox is rendered natively, not via the WebView",
			winerr.ErrInvalidOption)
	default:
		return ViewModel{}, winerr.Wrap(
			"dialogclient: unknown dialog type "+string(p.DialogType), winerr.ErrInvalidOption)
	}
}

// buildCloseApps assembles the close-applications dialog: a Continue/Install
// primary button, an optional Defer button, an optional Close-Apps button when
// applications are running, and the deferral / countdown chrome.
func buildCloseApps(vm ViewModel, o *ipc.CloseAppsOptions) (ViewModel, error) {
	if o == nil {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: CloseApps options missing",
			winerr.ErrInvalidOption,
		)
	}
	vm.Message = o.Message
	if o.CustomMessage != "" {
		vm.Message = o.CustomMessage
	}
	vm.Apps = make([]appVM, 0, len(o.Apps))
	for _, a := range o.Apps {
		vm.Apps = append(vm.Apps, appVM{Name: a.Name, Description: a.Description})
	}
	continueText := o.ButtonContinueText
	if continueText == "" {
		continueText = "Continue"
	}
	buttons := make([]buttonVM, 0, 3)
	if len(o.Apps) > 0 && o.ButtonCloseText != "" {
		buttons = append(
			buttons,
			buttonVM{ID: ButtonClose, Text: o.ButtonCloseText, Kind: "secondary"},
		)
	}
	if o.AllowDefer && o.ButtonDeferText != "" {
		buttons = append(
			buttons,
			buttonVM{ID: ButtonDefer, Text: o.ButtonDeferText, Kind: "secondary"},
		)
	}
	buttons = append(buttons, buttonVM{ID: ButtonContinue, Text: continueText, Kind: "primary"})
	vm.Buttons = buttons

	vm.ShowDeferral = o.AllowDefer
	vm.DeferralsRemaining = o.DeferralsRemaining
	vm.DeferralDeadline = o.DeferralDeadline
	if o.CountdownSeconds > 0 {
		vm.CountdownSeconds = o.CountdownSeconds
		vm.CountdownButton = ButtonContinue
	}
	return vm, nil
}

// buildCustom assembles the custom prompt (Show-ADTInstallationPrompt): up to
// three buttons mapped to Left/Middle/Right.
func buildCustom(vm ViewModel, o *ipc.CustomOptions) (ViewModel, error) {
	if o == nil {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: Custom options missing",
			winerr.ErrInvalidOption,
		)
	}
	vm.Message = o.Message
	vm.MessageAlign = o.MessageAlignment
	vm.Icon = o.Icon
	vm.Buttons = customButtons(o.ButtonLeftText, o.ButtonMiddleText, o.ButtonRightText)
	if len(vm.Buttons) == 0 {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: at least one button is required",
			winerr.ErrInvalidOption,
		)
	}
	return vm, nil
}

// buildInput assembles the input prompt with a single-line text control.
func buildInput(vm ViewModel, o *ipc.InputOptions) (ViewModel, error) {
	if o == nil {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: Input options missing",
			winerr.ErrInvalidOption,
		)
	}
	vm.Message = o.Message
	vm.Input = &inputVM{DefaultValue: o.DefaultValue, Placeholder: o.InputPlaceholder}
	vm.Buttons = leftRightButtons(o.ButtonLeftText, o.ButtonRightText)
	if len(vm.Buttons) == 0 {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: at least one button is required",
			winerr.ErrInvalidOption,
		)
	}
	return vm, nil
}

// buildList assembles the list-selection prompt.
func buildList(vm ViewModel, o *ipc.ListOptions) (ViewModel, error) {
	if o == nil {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: List options missing",
			winerr.ErrInvalidOption,
		)
	}
	if len(o.Items) == 0 {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: list requires at least one item",
			winerr.ErrInvalidOption,
		)
	}
	vm.Message = o.Message
	vm.List = &listVM{Items: o.Items, MultiSelect: o.MultiSelect}
	vm.Buttons = leftRightButtons(o.ButtonLeftText, o.ButtonRightText)
	if len(vm.Buttons) == 0 {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: at least one button is required",
			winerr.ErrInvalidOption,
		)
	}
	return vm, nil
}

// buildRestart assembles the restart prompt with a Restart-Now primary button,
// a Restart-Later button and an optional auto-restart countdown.
func buildRestart(vm ViewModel, o *ipc.RestartOptions) (ViewModel, error) {
	if o == nil {
		return ViewModel{}, winerr.Wrap(
			"dialogclient: Restart options missing",
			winerr.ErrInvalidOption,
		)
	}
	vm.Message = o.Message
	restartNow := o.ButtonRestartNowText
	if restartNow == "" {
		restartNow = "Restart Now"
	}
	buttons := make([]buttonVM, 0, 2)
	if o.ButtonRestartLater != "" {
		buttons = append(
			buttons,
			buttonVM{ID: ButtonRestartLater, Text: o.ButtonRestartLater, Kind: "secondary"},
		)
	}
	buttons = append(buttons, buttonVM{ID: ButtonRestartNow, Text: restartNow, Kind: "primary"})
	vm.Buttons = buttons
	if o.MessageRestart != "" {
		vm.Labels = map[string]string{"messageRestart": o.MessageRestart}
	}
	if o.CountdownSeconds > 0 {
		vm.CountdownSeconds = o.CountdownSeconds
		vm.CountdownButton = ButtonRestartNow
	}
	return vm, nil
}

// customButtons maps up to three labels onto Left/Middle/Right ids, marking the
// last present button primary (matching PSADT's rightmost-default convention).
func customButtons(left, middle, right string) []buttonVM {
	specs := []struct{ id, text string }{
		{ButtonLeft, left},
		{ButtonMiddle, middle},
		{ButtonRight, right},
	}
	out := make([]buttonVM, 0, 3)
	for _, s := range specs {
		if s.text != "" {
			out = append(out, buttonVM{ID: s.id, Text: s.text, Kind: "secondary"})
		}
	}
	if len(out) > 0 {
		out[len(out)-1].Kind = "primary"
	}
	return out
}

// leftRightButtons maps two labels onto Left/Right ids, the right one primary.
func leftRightButtons(left, right string) []buttonVM {
	out := make([]buttonVM, 0, 2)
	if left != "" {
		out = append(out, buttonVM{ID: ButtonLeft, Text: left, Kind: "secondary"})
	}
	if right != "" {
		out = append(out, buttonVM{ID: ButtonRight, Text: right, Kind: "primary"})
	}
	if len(out) == 1 {
		out[0].Kind = "primary"
	}
	return out
}

// ProgressView is the render-ready projection of an ipc.ProgressPayload for the
// modeless progress window. It is injected as window.__PROGRESS__ and refreshed
// in place via window.__setProgress(json).
type ProgressView struct {
	Title       string `json:"title"`
	Subtitle    string `json:"subtitle,omitempty"`
	Message     string `json:"message"`
	Detail      string `json:"detail,omitempty"`
	Percent     *int   `json:"percent"` // nil renders an indeterminate marquee
	AccentColor string `json:"accentColor,omitempty"`
	LogoImage   string `json:"logoImage,omitempty"`
	BannerImage string `json:"bannerImage,omitempty"`
}

// BuildProgressView projects an ipc.ProgressPayload onto a ProgressView,
// clamping any supplied percentage to [0,100].
func BuildProgressView(p ipc.ProgressPayload) ProgressView {
	v := ProgressView{
		Title:       p.Base.Title,
		Subtitle:    p.Base.Subtitle,
		Message:     p.StatusMessage,
		Detail:      p.StatusMessageDetail,
		AccentColor: p.Base.AccentColor,
		LogoImage:   p.Base.LogoImage,
		BannerImage: p.Base.BannerImage,
	}
	if p.ProgressPercent != nil {
		pct := *p.ProgressPercent
		if pct < 0 {
			pct = 0
		}
		if pct > 100 {
			pct = 100
		}
		v.Percent = &pct
	}
	return v
}

// JSON renders the progress view as the string injected into window.__PROGRESS__
// or passed to window.__setProgress.
func (v ProgressView) JSON() (string, error) {
	body, err := json.Marshal(v)
	if err != nil {
		return "", winerr.Wrap("dialogclient: marshaling progress view", winerr.ErrInvalidOption)
	}
	return string(body), nil
}

// FormatCountdown renders a whole-second countdown as HH:MM:SS, the caption
// shown by the Fluent countdown timer.
func FormatCountdown(seconds int) string {
	if seconds < 0 {
		seconds = 0
	}
	h := seconds / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}
