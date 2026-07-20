package manifest

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

// welcomeParams is the dialog.welcome parameter table (curated
// ShowADTInstallationWelcomeOptions surface).
var welcomeParams = []ParamSpec{
	{Name: "closeProcesses", Type: TypeProcessList, Description: "applications the user is asked to close"},
	{Name: "allowDefer", Type: TypeBool, Description: "offer deferral"},
	{Name: "deferTimes", Type: TypeInt, Description: "number of deferrals allowed"},
	{Name: "deferDeadline", Type: TypeTimestamp, Description: "date/time after which deferral is no longer offered"},
	{Name: "closeProcessesCountdown", Type: TypeInt, Description: "countdown seconds once deferral is exhausted"},
	{Name: "persistPrompt", Type: TypeBool, Description: "re-surface the prompt periodically"},
	{Name: "forceCloseProcessesCountdown", Type: TypeInt, Description: "countdown seconds that force-closes the apps"},
	{Name: "checkDiskSpace", Type: TypeBool, Description: "verify free disk space before continuing"},
	{Name: "requiredDiskSpace", Type: TypeInt, Description: "required disk space in MB (0 = auto-calculate)"},
	{Name: "minimizeWindows", Type: TypeBool, Description: "minimize other windows while the prompt shows"},
	{Name: "customText", Type: TypeBool, Description: "show the strings-table custom message"},
	{Name: "allowDeferCloseProcesses", Type: TypeBool, Description: "deferral offered; closing the apps continues immediately"},
	{Name: "deferRunInterval", Type: TypeDuration, Description: "minimum interval between welcome prompts"},
	{Name: "forceCountdown", Type: TypeInt, Description: "auto-continue countdown even while deferral is offered"},
	{Name: "promptToSave", Type: TypeBool, Description: "ask apps to close gracefully so users can save"},
	{Name: "blockExecution", Type: TypeBool, Description: "block the listed apps from restarting during the deployment"},
	{Name: "customMessageText", Type: TypeString, Description: "custom message shown beneath the dialog message"},
	{Name: "notTopMost", Type: TypeBool, Description: "render as a regular window instead of always-on-top"},
}

// checkWelcome applies dialog.welcome's cross-field rules.
func checkWelcome(p Params, add AddIssue) {
	deferControls := []string{"deferTimes", "deferDeadline", "deferRunInterval"}
	if !p.BoolOr("allowDefer", false) && !p.BoolOr("allowDeferCloseProcesses", false) {
		for _, name := range deferControls {
			if p.Has(name) {
				add(CodeSemantic, name, name+" has no effect without allowDefer or allowDeferCloseProcesses", true)
			}
		}
	}
	if p.Has("requiredDiskSpace") && !p.BoolOr("checkDiskSpace", false) {
		add(CodeSemantic, "requiredDiskSpace", "requiredDiskSpace has no effect without checkDiskSpace", true)
	}
	if !p.Has("closeProcesses") {
		for _, name := range []string{"promptToSave", "closeProcessesCountdown", "blockExecution", "forceCloseProcessesCountdown"} {
			if p.Has(name) {
				add(CodeSemantic, name, name+" has no effect without closeProcesses", true)
			}
		}
	}
}

func init() {
	register(StepSpec{
		Name: "dialog.welcome", Summary: "Close-apps/deferral welcome dialog",
		Platforms: []Platform{PlatformWindows},
		Params:    welcomeParams,
		Check:     checkWelcome,
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.ShowADTInstallationWelcomeOptions{
				AllowDefer:                   p.BoolOr("allowDefer", false),
				DeferTimes:                   p.IntOr("deferTimes", 0),
				CloseProcessesCountdown:      p.IntOr("closeProcessesCountdown", 0),
				PersistPrompt:                p.BoolOr("persistPrompt", false),
				ForceCloseProcessesCountdown: p.IntOr("forceCloseProcessesCountdown", 0),
				CheckDiskSpace:               p.BoolOr("checkDiskSpace", false),
				RequiredDiskSpace:            p.IntOr("requiredDiskSpace", 0),
				MinimizeWindows:              p.BoolOr("minimizeWindows", false),
				CustomText:                   p.BoolOr("customText", false),
				AllowDeferCloseProcesses:     p.BoolOr("allowDeferCloseProcesses", false),
				ForceCountdown:               p.IntOr("forceCountdown", 0),
				PromptToSave:                 p.BoolOr("promptToSave", false),
				BlockExecution:               p.BoolOr("blockExecution", false),
				CustomMessageText:            p.StringOr("customMessageText", ""),
				NotTopMost:                   p.BoolOr("notTopMost", false),
			}
			if v, ok := p.ProcessList("closeProcesses"); ok {
				opts.CloseProcesses = v
			}
			if v, ok := p.Time("deferDeadline"); ok {
				opts.DeferDeadline = v
			}
			if v, ok := p.Duration("deferRunInterval"); ok {
				opts.DeferRunInterval = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.ShowADTInstallationWelcome(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.progress", Summary: "Show or update the modeless progress dialog",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "statusMessage", Type: TypeString, Description: "main status text"},
			{Name: "statusMessageDetail", Type: TypeString, Description: "secondary detail text"},
			{Name: "progressPercentage", Type: TypeFloat, Description: "bar fill 0-100; negative renders a marquee"},
			{Name: "messageAlignment", Type: TypeString, Description: "status text alignment"},
			{Name: "windowLocation", Type: TypeString, Description: "window placement"},
		},
		Check: func(p Params, add AddIssue) {
			if v, ok := p.Float("progressPercentage"); ok && v > 100 {
				add(CodeSemantic, "progressPercentage", "progressPercentage must be at most 100", false)
			}
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.ShowADTInstallationProgressOptions{
				StatusMessage:       p.StringOr("statusMessage", ""),
				StatusMessageDetail: p.StringOr("statusMessageDetail", ""),
				ProgressPercentage:  -1,
				MessageAlignment:    p.StringOr("messageAlignment", ""),
				WindowLocation:      p.StringOr("windowLocation", ""),
			}
			if v, ok := p.Float("progressPercentage"); ok {
				opts.ProgressPercentage = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.ShowADTInstallationProgress(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.progressClose", Summary: "Close the progress dialog",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.CloseADTInstallationProgress(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.restartPrompt", Summary: "Prompt the user to restart the computer",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "countdownSeconds", Type: TypeInt, Description: "seconds until the restart is forced"},
			{Name: "countdownNoHideSeconds", Type: TypeInt, Description: "final seconds during which the prompt cannot be hidden"},
			{Name: "noCountdown", Type: TypeBool, Description: "show the prompt without a countdown"},
			{Name: "silentRestart", Type: TypeBool, Description: "restart without prompting in silent mode"},
			{Name: "silentBlockExecution", Type: TypeBool, Description: "keep blocked apps blocked through the silent restart"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.ShowADTInstallationRestartPromptOptions{
				CountdownSeconds:       p.IntOr("countdownSeconds", 0),
				CountdownNoHideSeconds: p.IntOr("countdownNoHideSeconds", 0),
				NoCountdown:            p.BoolOr("noCountdown", false),
				SilentRestart:          p.BoolOr("silentRestart", false),
				SilentBlockExecution:   p.BoolOr("silentBlockExecution", false),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.ShowADTInstallationRestartPrompt(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.prompt", Summary: "Custom prompt with up to three buttons",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "message", Type: TypeString, Required: true, Description: "prompt text"},
			{Name: "messageAlignment", Type: TypeString, Description: "text alignment"},
			{Name: "icon", Type: TypeString, Enum: []string{"information", "warning", "error", "question"},
				Description: "prompt icon"},
			{Name: "buttonLeftText", Type: TypeString, Description: "left button label"},
			{Name: "buttonMiddleText", Type: TypeString, Description: "middle button label"},
			{Name: "buttonRightText", Type: TypeString, Description: "right button label"},
			{Name: "noWait", Type: TypeBool, Description: "show without waiting for an answer"},
			{Name: "persistPrompt", Type: TypeBool, Description: "re-surface the prompt periodically"},
			{Name: "timeout", Type: TypeDuration, Description: "bound how long the prompt shows"},
		},
		Check: func(p Params, add AddIssue) {
			if !p.Has("buttonLeftText") && !p.Has("buttonMiddleText") && !p.Has("buttonRightText") {
				add(CodeSemantic, "", "dialog.prompt needs at least one button label", false)
			}
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.ShowADTInstallationPromptOptions{
				Message:          p.StringOr("message", ""),
				MessageAlignment: p.StringOr("messageAlignment", ""),
				Icon:             p.StringOr("icon", ""),
				ButtonLeftText:   p.StringOr("buttonLeftText", ""),
				ButtonMiddleText: p.StringOr("buttonMiddleText", ""),
				ButtonRightText:  p.StringOr("buttonRightText", ""),
				NoWait:           p.BoolOr("noWait", false),
				PersistPrompt:    p.BoolOr("persistPrompt", false),
			}
			if v, ok := p.Duration("timeout"); ok {
				opts.Timeout = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.ShowADTInstallationPrompt(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.dialogBox", Summary: "Classic message box",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "text", Type: TypeString, Required: true, Description: "message text"},
			{Name: "buttons", Type: TypeString,
				Enum:        []string{"ok", "okCancel", "yesNo", "yesNoCancel", "retryCancel", "abortRetryIgnore"},
				Description: "button set (default ok)"},
			{Name: "defaultButton", Type: TypeString, Description: "default button (First/Second/Third)"},
			{Name: "icon", Type: TypeString, Enum: []string{"information", "warning", "error", "question"},
				Description: "box icon"},
			{Name: "timeout", Type: TypeDuration, Description: "bound how long the box shows"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.ShowADTDialogBoxOptions{
				Text:          p.StringOr("text", ""),
				Buttons:       p.StringOr("buttons", ""),
				DefaultButton: p.StringOr("defaultButton", ""),
				Icon:          p.StringOr("icon", ""),
			}
			if v, ok := p.Duration("timeout"); ok {
				opts.Timeout = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.ShowADTDialogBox(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.balloonTip", Summary: "Tray balloon / toast notification",
		Platforms: []Platform{PlatformWindows},
		Params:    balloonParams,
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := balloonOptionsFromParams(p)
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.ShowADTBalloonTip(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.notifyIcon", Summary: "Persistent tray notification icon",
		Platforms: []Platform{PlatformWindows},
		Params:    balloonParams,
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := balloonOptionsFromParams(p)
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.ShowADTNotifyIcon(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "dialog.notifyIconClose", Summary: "Remove the tray notification icon",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.CloseADTNotifyIcon(ctx)
			}, nil
		},
	})
}

// balloonParams is shared by dialog.balloonTip and dialog.notifyIcon.
var balloonParams = []ParamSpec{
	{Name: "text", Type: TypeString, Required: true, Description: "notification text"},
	{Name: "title", Type: TypeString, Description: "notification title"},
	{Name: "icon", Type: TypeString, Enum: []string{"none", "info", "warning", "error"},
		Description: "notification icon (default info)"},
	{Name: "displayTime", Type: TypeDuration, Description: "how long the balloon shows"},
	{Name: "noWait", Type: TypeBool, Description: "do not block while the balloon shows"},
}

// balloonOptionsFromParams maps balloon params onto the option struct.
func balloonOptionsFromParams(p Params) adt.ShowADTBalloonTipOptions {
	opts := adt.ShowADTBalloonTipOptions{
		BalloonTipText:  p.StringOr("text", ""),
		BalloonTipTitle: p.StringOr("title", ""),
		BalloonTipIcon:  p.StringOr("icon", ""),
		NoWait:          p.BoolOr("noWait", false),
	}
	if v, ok := p.Duration("displayTime"); ok {
		opts.BalloonTipTime = v
	}
	return opts
}
