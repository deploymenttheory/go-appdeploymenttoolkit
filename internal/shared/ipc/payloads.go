package ipc

// This file defines the command-specific payload and result bodies carried in
// Request.Payload / Response.Result. They mirror PSADT's DialogOptions and
// DialogResults DTOs, reduced to the fields the Go dialogs render.

// DeploymentType echoes the session's verb so dialogs pick the right strings.
type DeploymentType string

// DeploymentType values.
const (
	DeploymentInstall   DeploymentType = "Install"
	DeploymentUninstall DeploymentType = "Uninstall"
	DeploymentRepair    DeploymentType = "Repair"
)

// BaseDialogOptions carries the chrome common to every dialog.
type BaseDialogOptions struct {
	Title          string         `json:"title"`
	Subtitle       string         `json:"subtitle"`
	DeploymentType DeploymentType `json:"deploymentType"`
	AppIconImage   string         `json:"appIconImage,omitempty"` // data: URI or path
	LogoImage      string         `json:"logoImage,omitempty"`
	BannerImage    string         `json:"bannerImage,omitempty"`
	AccentColor    string         `json:"accentColor,omitempty"` // #RRGGBB
	FluentStyle    bool           `json:"fluentStyle"`           // Fluent vs Classic
	TimeoutSeconds int            `json:"timeoutSeconds,omitempty"`
	// NotTopMost renders the dialog as a regular (non-always-on-top) window.
	NotTopMost bool `json:"notTopMost,omitempty"`
}

// ModalDialogPayload wraps the modal dialog request.
type ModalDialogPayload struct {
	DialogType DialogType        `json:"dialogType"`
	Base       BaseDialogOptions `json:"base"`
	CloseApps  *CloseAppsOptions `json:"closeApps,omitempty"`
	Custom     *CustomOptions    `json:"custom,omitempty"`
	Input      *InputOptions     `json:"input,omitempty"`
	List       *ListOptions      `json:"list,omitempty"`
	Restart    *RestartOptions   `json:"restart,omitempty"`
	Box        *DialogBoxOptions `json:"box,omitempty"`
}

// AppToClose describes one running application in the close-apps dialog.
type AppToClose struct {
	Name        string `json:"name"`        // process name, no extension
	Description string `json:"description"` // friendly name
}

// CloseAppsOptions mirrors CloseAppsDialogOptions.
type CloseAppsOptions struct {
	Message            string       `json:"message"`
	Apps               []AppToClose `json:"apps"`
	DeferralsRemaining int          `json:"deferralsRemaining"`
	DeferralDeadline   string       `json:"deferralDeadline,omitempty"`
	AllowDefer         bool         `json:"allowDefer"`
	CountdownSeconds   int          `json:"countdownSeconds,omitempty"`
	ButtonContinueText string       `json:"buttonContinueText"`
	ButtonDeferText    string       `json:"buttonDeferText"`
	ButtonCloseText    string       `json:"buttonCloseText"`
	CustomMessage      string       `json:"customMessage,omitempty"`
	// ForcedCountdown marks CountdownSeconds as a forced auto-continue
	// countdown (Show-ADTInstallationWelcome -ForceCountdown): it runs even
	// while deferral is offered and resolves to Continue.
	ForcedCountdown bool `json:"forcedCountdown,omitempty"`
	// ContinueOnProcessClosure relabels the flow for
	// -AllowDeferCloseProcesses: the prompt resolves as soon as the listed
	// apps are closed.
	ContinueOnProcessClosure bool `json:"continueOnProcessClosure,omitempty"`
}

// PromptToCloseAppsPayload asks the client to gracefully close every window
// of the named processes (WM_CLOSE, apps may show their own save prompts) and
// wait up to TimeoutSeconds for them to comply.
type PromptToCloseAppsPayload struct {
	ProcessNames   []string `json:"processNames"`
	TimeoutSeconds int      `json:"timeoutSeconds"`
}

// PromptToCloseAppsResult reports whether all windows closed in time.
type PromptToCloseAppsResult struct {
	AllClosed bool `json:"allClosed"`
}

// CustomOptions mirrors CustomDialogOptions (Show-ADTInstallationPrompt).
type CustomOptions struct {
	Message          string `json:"message"`
	MessageAlignment string `json:"messageAlignment,omitempty"`
	Icon             string `json:"icon,omitempty"` // Information/Warning/Error/Question
	ButtonLeftText   string `json:"buttonLeftText,omitempty"`
	ButtonMiddleText string `json:"buttonMiddleText,omitempty"`
	ButtonRightText  string `json:"buttonRightText,omitempty"`
}

// InputOptions mirrors InputDialogOptions.
type InputOptions struct {
	Message          string `json:"message"`
	DefaultValue     string `json:"defaultValue,omitempty"`
	InputPlaceholder string `json:"inputPlaceholder,omitempty"`
	ButtonLeftText   string `json:"buttonLeftText,omitempty"`
	ButtonRightText  string `json:"buttonRightText,omitempty"`
}

// ListOptions mirrors ListSelectionDialogOptions.
type ListOptions struct {
	Message         string   `json:"message"`
	Items           []string `json:"items"`
	MultiSelect     bool     `json:"multiSelect"`
	ButtonLeftText  string   `json:"buttonLeftText,omitempty"`
	ButtonRightText string   `json:"buttonRightText,omitempty"`
}

// RestartOptions mirrors RestartDialogOptions.
type RestartOptions struct {
	Message              string `json:"message"`
	MessageRestart       string `json:"messageRestart,omitempty"`
	CountdownSeconds     int    `json:"countdownSeconds,omitempty"`
	ButtonRestartNowText string `json:"buttonRestartNowText"`
	ButtonRestartLater   string `json:"buttonRestartLaterText"`
}

// DialogBoxOptions mirrors DialogBoxOptions (classic MessageBox).
type DialogBoxOptions struct {
	Text    string `json:"text"`
	Buttons string `json:"buttons"` // OK/OKCancel/YesNo/...
	Icon    string `json:"icon,omitempty"`
	Timeout int    `json:"timeout,omitempty"`
}

// ModalDialogResult wraps a modal dialog reply.
type ModalDialogResult struct {
	// Button is the pressed button ("Continue", "Defer", "Close", "Left",
	// "Middle", "Right", "RestartNow", "RestartLater", "OK", "Cancel",
	// "Yes", "No", "Timeout").
	Button string `json:"button"`
	// Input carries the InputDialog text.
	Input string `json:"input,omitempty"`
	// Selection carries the ListSelection chosen items.
	Selection []string `json:"selection,omitempty"`
}

// ProgressPayload mirrors ProgressDialogOptions (Show-ADTInstallationProgress).
type ProgressPayload struct {
	Base                BaseDialogOptions `json:"base"`
	StatusMessage       string            `json:"statusMessage"`
	StatusMessageDetail string            `json:"statusMessageDetail,omitempty"`
	ProgressPercent     *int              `json:"progressPercent,omitempty"` // nil = indeterminate
}

// BalloonPayload mirrors BalloonTipOptions.
type BalloonPayload struct {
	Title   string `json:"title"`
	Text    string `json:"text"`
	Icon    string `json:"icon,omitempty"` // Info/Warning/Error
	Timeout int    `json:"timeout,omitempty"`
}

// SendKeysPayload mirrors the SendKeys command.
type SendKeysPayload struct {
	WindowTitle string `json:"windowTitle"`
	Keys        string `json:"keys"`
}

// WindowInfoResult carries GetProcessWindowInfo results.
type WindowInfoResult struct {
	Windows []WindowInfo `json:"windows"`
}

// WindowInfo describes a top-level window.
type WindowInfo struct {
	Handle uint64 `json:"handle"`
	Title  string `json:"title"`
	PID    uint32 `json:"pid"`
}
