package winadt

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/strtab"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// Modal button identifiers returned by the dialog client.
const (
	dlgButtonContinue     = "Continue"
	dlgButtonDefer        = "Defer"
	dlgButtonClose        = "Close"
	dlgButtonRestartNow   = "RestartNow"
	dlgButtonRestartLater = "RestartLater"
	dlgButtonTimeout      = "Timeout"
)

// dialogState caches one dialog server for the process lifetime.
//
// Design note: the Session type cannot be extended (it is owned by another
// package), so the server is held here, keyed to the process rather than to a
// session. A single deployment process drives a single interactive session, so
// one cached server is correct; it is built lazily on the first UI call and
// reused (the progress window in particular is stateful across calls). A failed
// build is not cached, so a later call may retry.
var dialogState struct {
	mu  sync.Mutex
	srv *dialogserver.DialogServer
}

// acquireDialogServer returns the cached dialog server, building it on first
// use. It is only reached in interactive/non-silent flows.
func acquireDialogServer(ctx context.Context, s *DeploymentSession) (*dialogserver.DialogServer, error) {
	dialogState.mu.Lock()
	defer dialogState.mu.Unlock()
	if dialogState.srv != nil {
		return dialogState.srv, nil
	}
	srv, err := newDialogServer(ctx, s)
	if err != nil {
		return nil, err
	}
	dialogState.srv = srv
	return srv, nil
}

// CloseADTDialogServer tears down the cached dialog server (closing the client
// process / releasing the WebView). It is safe to call when none was built.
func CloseADTDialogServer() {
	dialogState.mu.Lock()
	srv := dialogState.srv
	dialogState.srv = nil
	dialogState.mu.Unlock()
	if srv != nil {
		srv.Close()
	}
}

// sessionString resolves a string-table entry for the session's deployment type
// and interpolates any {Section\Key} config references.
func sessionString(s *DeploymentSession, path string) string {
	raw := s.Strings().MustGet(path, s.DeploymentType().String())
	return strtab.Interpolate(raw, s.Config().Lookup)
}

// fluentAccentColor formats the config accent color as #RRGGBB, or "" when
// unset so the dialog uses its built-in default.
func fluentAccentColor(cfg accentConfig) string {
	rgb := cfg & 0xFFFFFF
	if rgb == 0 {
		return ""
	}
	return fmt.Sprintf("#%06X", rgb)
}

// accentConfig is the numeric accent color type (config UI.FluentAccentColor).
type accentConfig = uint32

// defaultDialogTimeout is the configured UI.DefaultTimeout as a duration.
func defaultDialogTimeout(s *DeploymentSession) time.Duration {
	return time.Duration(s.Config().UI.DefaultTimeout) * time.Second
}

// baseOptions builds the chrome common to every dialog.
func baseOptions(s *DeploymentSession, subtitlePath string, timeout time.Duration) ipc.BaseDialogOptions {
	cfg := s.Config()
	b := ipc.BaseDialogOptions{
		Title:          s.InstallTitle(),
		DeploymentType: ipc.DeploymentType(s.DeploymentType().String()),
		FluentStyle:    strings.EqualFold(cfg.UI.DialogStyle, "Fluent"),
		AccentColor:    fluentAccentColor(cfg.UI.FluentAccentColor),
		LogoImage:      assetDataURI(cfg.Assets.Logo),
		BannerImage:    assetDataURI(cfg.Assets.Banner),
	}
	if subtitlePath != "" {
		b.Subtitle = sessionString(s, subtitlePath)
	}
	if timeout > 0 {
		b.TimeoutSeconds = int(timeout.Seconds())
	}
	return b
}

// assetDataURI turns a config asset value into an inline data: URI. A value
// already in data:/https: form is passed through; an existing file path is read
// and base64-encoded; anything else yields "" (no image).
func assetDataURI(val string) string {
	if val == "" || strings.HasPrefix(val, "data:") || strings.HasPrefix(val, "http") {
		return val
	}
	info, err := os.Stat(val)
	if err != nil || info.IsDir() {
		return ""
	}
	data, err := os.ReadFile(val) //#nosec G304 -- path comes from the trusted package config, not user input
	if err != nil {
		return ""
	}
	mime := "image/png"
	switch strings.ToLower(filepath.Ext(val)) {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".ico":
		mime = "image/x-icon"
	case ".svg":
		mime = "image/svg+xml"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
}

// terminateProcesses force-closes the given running processes (best-effort).
func terminateProcesses(running []RunningProcess) {
	for _, p := range running {
		proc, err := os.FindProcess(int(p.PID))
		if err != nil {
			continue
		}
		_ = proc.Kill()
	}
}

// ShowADTInstallationWelcomeOptions mirrors the parameters of
// Show-ADTInstallationWelcome.
type ShowADTInstallationWelcomeOptions struct {
	CloseProcesses               []ProcessObject
	AllowDefer                   bool
	DeferTimes                   int
	DeferDeadline                time.Time
	CloseProcessesCountdown      int
	PersistPrompt                bool
	ForceCloseProcessesCountdown int
	CheckDiskSpace               bool
	RequiredDiskSpace            int
	MinimizeWindows              bool
	CustomText                   bool
	// AllowDeferCloseProcesses implies AllowDefer and resolves the prompt
	// as soon as the listed applications are no longer running.
	AllowDeferCloseProcesses bool
	// DeferRunInterval is the minimum interval between welcome prompts:
	// while it has not elapsed since the last prompt, the call defers
	// immediately without consuming a deferral.
	DeferRunInterval time.Duration
	// ForceCountdown makes the dialog auto-continue after this many
	// seconds even while deferral is on offer.
	ForceCountdown int
	// PromptToSave asks the running applications to close gracefully
	// (WM_CLOSE, so they can prompt the user to save) before any forced
	// termination, waiting config UI.PromptToSaveTimeout.
	PromptToSave bool
	// BlockExecution blocks the listed applications from restarting while
	// the deployment runs; the block lifts automatically at session close.
	BlockExecution bool
	// CustomMessageText overrides the strings-table custom message shown
	// beneath the dialog message (CustomText picks the strings-table one).
	CustomMessageText string
	// NotTopMost renders the dialog as a regular window instead of
	// always-on-top.
	NotTopMost bool
}

// WelcomeResult reports how the welcome prompt resolved.
type WelcomeResult struct {
	// Action is the terminal button ("Continue", "Close", "Defer", "Timeout").
	Action string
	// Deferred is true when the user chose to defer.
	Deferred bool
}

// deferralState is the resolved deferral decision (portable, unit-tested).
type deferralState struct {
	Allowed     bool
	Remaining   int
	HasDeadline bool
	Deadline    time.Time
	Expired     bool
}

// computeDeferralState resolves whether deferral is allowed and how many
// deferrals remain, honoring persisted history and an optional deadline.
//
// Deviation from PSADT: the "remaining" count is the plain number of deferrals
// still available (history value when present, else DeferTimes); PSADT's exact
// pre-decrement off-by-one is not reproduced.
func computeDeferralState(
	allowDefer bool,
	deferTimes int,
	histTimes *uint32,
	deadline time.Time,
	now time.Time,
) deferralState {
	st := deferralState{}
	if !allowDefer {
		return st
	}
	st.Allowed = true
	base := deferTimes
	haveTimes := deferTimes > 0 || histTimes != nil
	if histTimes != nil {
		base = int(*histTimes)
	}
	if haveTimes {
		st.Remaining = base
		if base <= 0 {
			st.Allowed = false
			st.Expired = true
		}
	}
	if !deadline.IsZero() {
		st.HasDeadline = true
		st.Deadline = deadline
		if now.After(deadline) {
			st.Allowed = false
			st.Expired = true
		}
	}
	return st
}

// ShowADTInstallationWelcome is the Go port of Show-ADTInstallationWelcome. In
// silent/non-interactive modes it force-closes the listed processes and
// proceeds; otherwise it presents the close-apps dialog, looping until the
// processes are closed, the countdown elapses, or the user defers. A defer
// returns an error wrapping ErrDeferred (the runner maps it to DeferExitCode).
func ShowADTInstallationWelcome(
	ctx context.Context,
	opts ShowADTInstallationWelcomeOptions,
) (WelcomeResult, error) {
	if err := ctx.Err(); err != nil {
		return WelcomeResult{}, fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return WelcomeResult{}, err
	}
	logToSession("Evaluating installation welcome prompt.", LogSeverityInfo, "ShowADTInstallationWelcome")
	opts.AllowDefer = opts.AllowDefer || opts.AllowDeferCloseProcesses

	running, _ := GetADTRunningProcesses(ctx, opts.CloseProcesses)

	if s.IsSilent() || s.IsNonInteractive() {
		if len(running) > 0 {
			logToSession("Silent mode: force-closing running applications.", LogSeverityInfo, "ShowADTInstallationWelcome")
			terminateProcesses(running)
		}
		return WelcomeResult{Action: dlgButtonContinue}, nil
	}

	hist, _ := s.DeferHistory()
	now := time.Now()
	state := computeDeferralState(opts.AllowDefer, opts.DeferTimes, hist.TimesRemaining, opts.DeferDeadline, now)

	// DeferRunInterval: within the interval since the last prompt, defer
	// again immediately without consuming a deferral (PSADT parity). Only
	// applies while deferral is still on offer.
	if state.Allowed && !deferRunIntervalDue(hist.RunIntervalLastTime, opts.DeferRunInterval, now) {
		logToSession(fmt.Sprintf(
			"Deferring: the next welcome prompt is not due until [%s].",
			hist.RunIntervalLastTime.Add(opts.DeferRunInterval).Format(time.RFC3339)),
			LogSeverityInfo, "ShowADTInstallationWelcome")
		fireOnDefer(ctx, s)
		return WelcomeResult{Action: dlgButtonDefer, Deferred: true},
			fmt.Errorf("adt: welcome deferred (run interval): %w", ErrDeferred)
	}

	if len(running) == 0 && !state.Allowed {
		return WelcomeResult{Action: dlgButtonContinue}, nil
	}

	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		// No interactive UI reachable: behave like silent and proceed.
		logToSession("No interactive session for welcome prompt; proceeding: "+err.Error(),
			LogSeverityWarning, "ShowADTInstallationWelcome")
		terminateProcesses(running)
		return WelcomeResult{Action: dlgButtonContinue}, nil
	}
	if opts.MinimizeWindows {
		_ = srv.MinimizeWindows(ctx)
	}
	res, err := runWelcomeLoop(ctx, s, srv, opts, state)
	if err == nil && opts.BlockExecution && len(opts.CloseProcesses) > 0 {
		// Proceeding with the deployment: block the apps from restarting and
		// lift the block automatically at session close (PSADT clears the
		// block on Defer/Timeout, which return errors and skip this).
		names := make([]string, len(opts.CloseProcesses))
		for i, p := range opts.CloseProcesses {
			names[i] = p.Name
		}
		if blockErr := BlockADTAppExecution(ctx, names); blockErr != nil {
			logToSession("Failed to block application execution: "+blockErr.Error(),
				LogSeverityWarning, "ShowADTInstallationWelcome")
		} else {
			AddADTSessionClosingCallback(s, func(ctx context.Context, s *DeploymentSession) error {
				return UnblockADTAppExecution(ctx)
			})
		}
	}
	return res, err
}

// deferRunIntervalDue reports whether enough time has passed since the last
// welcome prompt for a new one (true when no interval or no history).
func deferRunIntervalDue(last *time.Time, interval time.Duration, now time.Time) bool {
	if interval <= 0 || last == nil {
		return true
	}
	return !now.Before(last.Add(interval))
}

// runWelcomeLoop drives the close-apps dialog until resolution.
func runWelcomeLoop(
	ctx context.Context,
	s *DeploymentSession,
	srv *dialogserver.DialogServer,
	opts ShowADTInstallationWelcomeOptions,
	state deferralState,
) (WelcomeResult, error) {
	for {
		if err := ctx.Err(); err != nil {
			return WelcomeResult{}, fmt.Errorf("adt: %w", err)
		}
		running, _ := GetADTRunningProcesses(ctx, opts.CloseProcesses)
		if len(running) == 0 && (!state.Allowed || opts.AllowDeferCloseProcesses) {
			// All apps are closed: proceed. With AllowDeferCloseProcesses the
			// closure alone resolves the prompt even while deferral remains.
			return WelcomeResult{Action: dlgButtonContinue}, nil
		}
		payload := buildCloseAppsPayload(s, opts, running, state)
		res, err := srv.ShowModal(ctx, payload)
		if err != nil {
			return WelcomeResult{}, err
		}
		switch res.Button {
		case dlgButtonDefer:
			persistDeferral(s, state, opts.DeferRunInterval)
			logToSession("User deferred the installation.", LogSeverityInfo, "ShowADTInstallationWelcome")
			fireOnDefer(ctx, s)
			return WelcomeResult{Action: dlgButtonDefer, Deferred: true},
				fmt.Errorf("adt: welcome deferred: %w", ErrDeferred)
		case dlgButtonClose, dlgButtonContinue, dlgButtonTimeout:
			if res.Button == dlgButtonTimeout {
				// A timed-out prompt also anchors the run interval (PSADT
				// updates DeferRunIntervalLastTime on timeout).
				persistRunInterval(s, opts.DeferRunInterval)
			}
			running, _ = GetADTRunningProcesses(ctx, opts.CloseProcesses)
			if len(running) > 0 {
				if opts.PromptToSave {
					gracefulCloseProcesses(ctx, s, srv, running)
					running, _ = GetADTRunningProcesses(ctx, opts.CloseProcesses)
				}
			}
			if len(running) > 0 {
				logToSession("Closing running applications to continue.", LogSeverityInfo, "ShowADTInstallationWelcome")
				terminateProcesses(running)
				waitForProcessExit(ctx, opts.CloseProcesses)
			}
			if remaining, _ := GetADTRunningProcesses(ctx, opts.CloseProcesses); len(remaining) == 0 {
				return WelcomeResult{Action: res.Button}, nil
			}
		default:
			return WelcomeResult{Action: res.Button}, nil
		}
	}
}

// gracefulCloseProcesses asks the dialog client to close the running apps'
// windows via WM_CLOSE (so they can prompt to save), waiting the configured
// UI.PromptToSaveTimeout. Failures degrade to the force-terminate path.
func gracefulCloseProcesses(
	ctx context.Context,
	s *DeploymentSession,
	srv *dialogserver.DialogServer,
	running []RunningProcess,
) {
	names := make([]string, len(running))
	for i, p := range running {
		names[i] = p.Name
	}
	logToSession("Prompting the running applications to close and save their work.",
		LogSeverityInfo, "ShowADTInstallationWelcome")
	res, err := srv.PromptToCloseApps(ctx, ipc.PromptToCloseAppsPayload{
		ProcessNames:   names,
		TimeoutSeconds: s.Config().UI.PromptToSaveTimeout,
	})
	switch {
	case err != nil:
		logToSession("Graceful close failed: "+err.Error(), LogSeverityWarning, "ShowADTInstallationWelcome")
	case !res.AllClosed:
		logToSession("Not all application windows closed within the PromptToSaveTimeout.",
			LogSeverityWarning, "ShowADTInstallationWelcome")
	}
}

// waitForProcessExit polls up to five seconds for the processes to disappear.
func waitForProcessExit(ctx context.Context, procs []ProcessObject) {
	for i := 0; i < 5; i++ {
		if err := ctx.Err(); err != nil {
			return
		}
		if running, _ := GetADTRunningProcesses(ctx, procs); len(running) == 0 {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

// fireOnDefer invokes the session's OnDefer hooks (PSADT OnDefer callback).
func fireOnDefer(ctx context.Context, s *DeploymentSession) {
	for _, h := range deploy.HooksOf(s).OnDefer {
		h(ctx, s)
	}
}

// persistDeferral writes the decremented deferral history after a defer,
// anchoring the run interval when one is configured.
func persistDeferral(s *DeploymentSession, state deferralState, runInterval time.Duration) {
	h, _ := s.DeferHistory()
	if state.Remaining > 0 {
		next := uint32(state.Remaining - 1) //#nosec G115 -- Remaining is a small non-negative count
		h.TimesRemaining = &next
	}
	if state.HasDeadline {
		d := state.Deadline
		h.Deadline = &d
	}
	if runInterval > 0 {
		now := time.Now()
		h.RunInterval = &runInterval
		h.RunIntervalLastTime = &now
	}
	if err := s.SetDeferHistory(h); err != nil {
		logToSession("Failed to persist deferral history: "+err.Error(), LogSeverityWarning, "ShowADTInstallationWelcome")
	}
}

// persistRunInterval anchors only the run-interval timestamp (used when a
// prompt times out without an explicit defer).
func persistRunInterval(s *DeploymentSession, runInterval time.Duration) {
	if runInterval <= 0 {
		return
	}
	h, _ := s.DeferHistory()
	now := time.Now()
	h.RunInterval = &runInterval
	h.RunIntervalLastTime = &now
	if err := s.SetDeferHistory(h); err != nil {
		logToSession("Failed to persist deferral history: "+err.Error(), LogSeverityWarning, "ShowADTInstallationWelcome")
	}
}

// buildCloseAppsPayload assembles the close-apps modal from session strings and
// config, honoring the deferral decision and any countdown.
func buildCloseAppsPayload(
	s *DeploymentSession,
	opts ShowADTInstallationWelcomeOptions,
	running []RunningProcess,
	state deferralState,
) ipc.ModalDialogPayload {
	cfg := s.Config()
	fluent := strings.EqualFold(cfg.UI.DialogStyle, "Fluent")
	prefix := "CloseAppsPrompt.Classic"
	subtitlePath := ""
	if fluent {
		prefix = "CloseAppsPrompt.Fluent"
		subtitlePath = "CloseAppsPrompt.Fluent.Subtitle"
	}

	apps := make([]ipc.AppToClose, 0, len(running))
	for _, p := range running {
		apps = append(apps, ipc.AppToClose{Name: p.Name, Description: p.Description})
	}

	message := sessionString(s, prefix+".DialogMessage")
	if len(running) == 0 {
		if msg := tryString(s, prefix+".DialogMessageNoProcesses"); msg != "" {
			message = msg
		}
	}
	continueText := sessionString(s, closeAppsContinueKey(prefix, len(running) == 0))
	closeText := tryString(s, prefix+".ButtonLeftText")
	deferText := tryString(s, prefix+".ButtonRightText")

	co := &ipc.CloseAppsOptions{
		Message:                  message,
		Apps:                     apps,
		AllowDefer:               state.Allowed,
		DeferralsRemaining:       state.Remaining,
		ButtonContinueText:       continueText,
		ButtonDeferText:          deferText,
		ButtonCloseText:          closeText,
		CountdownSeconds:         welcomeCountdown(opts, state),
		ForcedCountdown:          forcedWelcomeCountdown(opts, state),
		ContinueOnProcessClosure: opts.AllowDeferCloseProcesses,
	}
	switch {
	case opts.CustomMessageText != "":
		co.CustomMessage = opts.CustomMessageText
	case opts.CustomText:
		co.CustomMessage = tryString(s, prefix+".CustomMessage")
	}
	if state.HasDeadline {
		co.DeferralDeadline = state.Deadline.Format("2006-01-02 15:04")
	}
	base := baseOptions(s, subtitlePath, defaultDialogTimeout(s))
	base.NotTopMost = opts.NotTopMost
	return ipc.ModalDialogPayload{
		DialogType: ipc.DialogCloseApps,
		Base:       base,
		CloseApps:  co,
	}
}

// closeAppsContinueKey selects the continue/install button string.
func closeAppsContinueKey(prefix string, noProcesses bool) string {
	if noProcesses {
		return prefix + ".ButtonLeftNoProcessesText"
	}
	return prefix + ".ButtonLeftText"
}

// welcomeCountdown resolves the auto-continue countdown seconds: forced
// close-countdown first, then ForceCountdown (even during deferral), then the
// plain countdown once deferral is exhausted.
func welcomeCountdown(opts ShowADTInstallationWelcomeOptions, state deferralState) int {
	if opts.ForceCloseProcessesCountdown > 0 {
		return opts.ForceCloseProcessesCountdown
	}
	if opts.ForceCountdown > 0 {
		return opts.ForceCountdown
	}
	if !state.Allowed && opts.CloseProcessesCountdown > 0 {
		return opts.CloseProcessesCountdown
	}
	return 0
}

// forcedWelcomeCountdown reports whether the countdown is a forced
// auto-continue one (runs even while deferral is on offer).
func forcedWelcomeCountdown(opts ShowADTInstallationWelcomeOptions, state deferralState) bool {
	if opts.ForceCloseProcessesCountdown > 0 {
		return true
	}
	return opts.ForceCountdown > 0 && state.Allowed
}

// tryString resolves a string-table entry, returning "" when it is absent
// (MustGet echoes the path back for missing keys).
func tryString(s *DeploymentSession, path string) string {
	if raw, ok := s.Strings().Get(path, s.DeploymentType().String()); ok {
		return strtab.Interpolate(raw, s.Config().Lookup)
	}
	return ""
}

// ShowADTInstallationProgressOptions mirrors Show-ADTInstallationProgress.
type ShowADTInstallationProgressOptions struct {
	StatusMessage       string
	StatusMessageDetail string
	// ProgressPercentage is the bar fill in [0,100]; a negative value renders
	// an indeterminate marquee.
	ProgressPercentage float64
	MessageAlignment   string
	WindowLocation     string
}

// ShowADTInstallationProgress is the Go port of Show-ADTInstallationProgress: a
// modeless progress window, updated in place on subsequent calls. Silent mode
// logs and returns.
func ShowADTInstallationProgress(ctx context.Context, opts ShowADTInstallationProgressOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	if s.IsSilent() {
		logToSession("Bypassing progress dialog (silent mode).", LogSeverityInfo, "ShowADTInstallationProgress")
		return nil
	}
	message := opts.StatusMessage
	if message == "" {
		message = sessionString(s, "ProgressPrompt.Message")
	}
	detail := opts.StatusMessageDetail
	if detail == "" {
		detail = tryString(s, "ProgressPrompt.MessageDetail")
	}
	payload := ipc.ProgressPayload{
		Base:                baseOptions(s, "ProgressPrompt.Subtitle", 0),
		StatusMessage:       message,
		StatusMessageDetail: detail,
	}
	if opts.ProgressPercentage >= 0 {
		pct := int(opts.ProgressPercentage)
		payload.ProgressPercent = &pct
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		logToSession("No interactive session for progress dialog: "+err.Error(), LogSeverityWarning, "ShowADTInstallationProgress")
		return nil
	}
	return srv.ShowProgress(ctx, payload)
}

// CloseADTInstallationProgress is the Go port of Close-ADTInstallationProgress.
func CloseADTInstallationProgress(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	if s.IsSilent() {
		return nil
	}
	dialogState.mu.Lock()
	srv := dialogState.srv
	dialogState.mu.Unlock()
	if srv == nil {
		return nil
	}
	return srv.CloseProgress(ctx)
}

// ShowADTInstallationPromptOptions mirrors Show-ADTInstallationPrompt.
type ShowADTInstallationPromptOptions struct {
	Message          string
	MessageAlignment string
	Icon             string
	ButtonLeftText   string
	ButtonMiddleText string
	ButtonRightText  string
	NoWait           bool
	PersistPrompt    bool
	Timeout          time.Duration
}

// ShowADTInstallationPrompt is the Go port of Show-ADTInstallationPrompt. It
// returns the pressed button text. Non-interactive mode returns "". NoWait
// shows the prompt on a background goroutine and returns immediately.
func ShowADTInstallationPrompt(ctx context.Context, opts ShowADTInstallationPromptOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	if s.IsSilent() || s.IsNonInteractive() {
		logToSession("Bypassing installation prompt (silent/non-interactive).", LogSeverityInfo, "ShowADTInstallationPrompt")
		return "", nil
	}
	if opts.ButtonLeftText == "" && opts.ButtonMiddleText == "" && opts.ButtonRightText == "" {
		opts.ButtonRightText = "OK"
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultDialogTimeout(s)
	}
	payload := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCustom,
		Base:       baseOptions(s, "InstallationPrompt.Subtitle", timeout),
		Custom: &ipc.CustomOptions{
			Message:          opts.Message,
			MessageAlignment: opts.MessageAlignment,
			Icon:             opts.Icon,
			ButtonLeftText:   opts.ButtonLeftText,
			ButtonMiddleText: opts.ButtonMiddleText,
			ButtonRightText:  opts.ButtonRightText,
		},
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		logToSession("No interactive session for prompt: "+err.Error(), LogSeverityWarning, "ShowADTInstallationPrompt")
		return "", nil
	}
	if opts.NoWait {
		go func() {
			if _, err := srv.ShowModal(context.WithoutCancel(ctx), payload); err != nil {
				logToSession("NoWait prompt failed: "+err.Error(), LogSeverityWarning, "ShowADTInstallationPrompt")
			}
		}()
		return "", nil
	}
	res, err := srv.ShowModal(ctx, payload)
	if err != nil {
		return "", err
	}
	return buttonText(opts, res.Button), nil
}

// buttonText maps a modal button id back to the caller's button label.
func buttonText(opts ShowADTInstallationPromptOptions, button string) string {
	switch button {
	case "Left":
		return opts.ButtonLeftText
	case "Middle":
		return opts.ButtonMiddleText
	case "Right":
		return opts.ButtonRightText
	default:
		return button
	}
}

// ShowADTInstallationRestartPromptOptions mirrors Show-ADTInstallationRestartPrompt.
type ShowADTInstallationRestartPromptOptions struct {
	CountdownSeconds       int
	CountdownNoHideSeconds int
	NoCountdown            bool
	SilentRestart          bool
	SilentBlockExecution   bool
}

// ShowADTInstallationRestartPrompt is the Go port of
// Show-ADTInstallationRestartPrompt. It shows the restart dialog and, when the
// user chooses Restart Now or the countdown elapses, initiates the restart.
// Silent mode with SilentRestart restarts after a short delay.
func ShowADTInstallationRestartPrompt(
	ctx context.Context,
	opts ShowADTInstallationRestartPromptOptions,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	if s.IsSilent() {
		if opts.SilentRestart {
			logToSession("Silent restart requested; restarting shortly.", LogSeverityInfo, "ShowADTInstallationRestartPrompt")
			return dlgButtonRestartNow, initiateSystemRestart(ctx, 5, sessionString(s, "RestartPrompt.Message"))
		}
		return "", nil
	}
	countdown := opts.CountdownSeconds
	if opts.NoCountdown {
		countdown = 0
	} else if countdown == 0 {
		countdown = 60
	}
	payload := ipc.ModalDialogPayload{
		DialogType: ipc.DialogRestart,
		Base:       baseOptions(s, "RestartPrompt.Subtitle", 0),
		Restart: &ipc.RestartOptions{
			Message:              sessionString(s, "RestartPrompt.Message"),
			MessageRestart:       tryString(s, "RestartPrompt.MessageRestart"),
			CountdownSeconds:     countdown,
			ButtonRestartNowText: tryString(s, "RestartPrompt.ButtonRestartNow"),
			ButtonRestartLater:   tryString(s, "RestartPrompt.ButtonRestartLater"),
		},
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		logToSession("No interactive session for restart prompt: "+err.Error(), LogSeverityWarning, "ShowADTInstallationRestartPrompt")
		return "", nil
	}
	res, err := srv.ShowModal(ctx, payload)
	if err != nil {
		return "", err
	}
	if res.Button == dlgButtonRestartNow || res.Button == dlgButtonTimeout {
		logToSession("Initiating system restart.", LogSeverityInfo, "ShowADTInstallationRestartPrompt")
		if rerr := initiateSystemRestart(ctx, 10, sessionString(s, "RestartPrompt.Message")); rerr != nil {
			return res.Button, rerr
		}
	}
	return res.Button, nil
}

// ShowADTDialogBoxOptions mirrors Show-ADTDialogBox.
type ShowADTDialogBoxOptions struct {
	Text          string
	Buttons       string
	DefaultButton string
	Icon          string
	Timeout       time.Duration
}

// ShowADTDialogBox is the Go port of Show-ADTDialogBox: a classic MessageBox.
// It returns the pressed button text. Non-interactive mode returns "".
func ShowADTDialogBox(ctx context.Context, opts ShowADTDialogBoxOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	if s.IsSilent() || s.IsNonInteractive() {
		return "", nil
	}
	buttons := opts.Buttons
	if buttons == "" {
		buttons = "OK"
	}
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = defaultDialogTimeout(s)
	}
	payload := ipc.ModalDialogPayload{
		DialogType: ipc.DialogBox,
		Base:       baseOptions(s, "", timeout),
		Box: &ipc.DialogBoxOptions{
			Text:    opts.Text,
			Buttons: buttons,
			Icon:    opts.Icon,
			Timeout: int(timeout.Seconds()),
		},
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		logToSession("No interactive session for dialog box: "+err.Error(), LogSeverityWarning, "ShowADTDialogBox")
		return "", nil
	}
	res, err := srv.ShowModal(ctx, payload)
	if err != nil {
		return "", err
	}
	return res.Button, nil
}

// ShowADTBalloonTipOptions mirrors Show-ADTBalloonTip.
type ShowADTBalloonTipOptions struct {
	BalloonTipText  string
	BalloonTipTitle string
	BalloonTipIcon  string
	BalloonTipTime  time.Duration
	NoWait          bool
}

// ShowADTBalloonTip is the Go port of Show-ADTBalloonTip: a tray balloon /
// toast. It is skipped when balloon notifications are disabled or in silent
// mode.
func ShowADTBalloonTip(ctx context.Context, opts ShowADTBalloonTipOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	if !s.Config().UI.BalloonNotifications {
		logToSession("Balloon notifications are disabled; skipping.", LogSeverityInfo, "ShowADTBalloonTip")
		return nil
	}
	if s.IsSilent() {
		logToSession("Bypassing balloon tip (silent mode).", LogSeverityInfo, "ShowADTBalloonTip")
		return nil
	}
	title := opts.BalloonTipTitle
	if title == "" {
		title = s.InstallTitle()
	}
	payload := ipc.BalloonPayload{
		Title: title,
		Text:  opts.BalloonTipText,
		Icon:  opts.BalloonTipIcon,
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		logToSession("No interactive session for balloon tip: "+err.Error(), LogSeverityWarning, "ShowADTBalloonTip")
		return nil
	}
	return srv.ShowBalloon(ctx, payload)
}

// ShowADTNotifyIcon shows or updates the tray notification icon by surfacing a
// balloon; it shares the balloon implementation.
func ShowADTNotifyIcon(ctx context.Context, opts ShowADTBalloonTipOptions) error {
	return ShowADTBalloonTip(ctx, opts)
}

// CloseADTNotifyIcon removes the tray notification icon by tearing down the
// dialog server that owns it.
func CloseADTNotifyIcon(_ context.Context) error {
	CloseADTDialogServer()
	return nil
}

// SendADTKeysOptions mirrors Send-ADTKeys.
type SendADTKeysOptions struct {
	WindowTitle string
	Keys        string
	WaitSeconds int
}

// SendADTKeys is the Go port of Send-ADTKeys: it sends a keystroke sequence to
// the window(s) matching WindowTitle.
func SendADTKeys(ctx context.Context, opts SendADTKeysOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		return err
	}
	if err := srv.SendKeys(ctx, ipc.SendKeysPayload{WindowTitle: opts.WindowTitle, Keys: opts.Keys}); err != nil {
		return err
	}
	if opts.WaitSeconds > 0 {
		select {
		case <-ctx.Done():
			return fmt.Errorf("adt: %w", ctx.Err())
		case <-time.After(time.Duration(opts.WaitSeconds) * time.Second):
		}
	}
	return nil
}

// UpdateADTDesktop is the Go port of Update-ADTDesktop: it refreshes the
// desktop and reloads environment variables in the interactive session.
func UpdateADTDesktop(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	srv, err := acquireDialogServer(ctx, s)
	if err != nil {
		return err
	}
	return srv.RefreshDesktop(ctx)
}

// BlockADTAppExecution is the Go port of Block-ADTAppExecution: it installs
// Image File Execution Options "Debugger" hooks so the named processes launch
// a block message instead, and registers a scheduled task that unblocks them
// on next logon should the deployment be interrupted.
func BlockADTAppExecution(ctx context.Context, processNames []string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	if len(processNames) == 0 {
		return winerr.Wrap("adt: at least one process name is required", winerr.ErrInvalidOption)
	}
	return blockAppExecution(ctx, s, processNames)
}

// UnblockADTAppExecution is the Go port of Unblock-ADTAppExecution: it removes
// the IFEO Debugger hooks and the cleanup scheduled task.
func UnblockADTAppExecution(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	return unblockAppExecution(ctx, s)
}

// UserNotificationState mirrors QUERY_USER_NOTIFICATION_STATE.
type UserNotificationState int

// UserNotificationState values (match the Win32 enum ordinals).
const (
	UserNotificationStateNotPresent           UserNotificationState = 1
	UserNotificationStateBusy                 UserNotificationState = 2
	UserNotificationStateRunningD3DFullScreen UserNotificationState = 3
	UserNotificationStatePresentationMode     UserNotificationState = 4
	UserNotificationStateAcceptsNotifications UserNotificationState = 5
	UserNotificationStateQuietTime            UserNotificationState = 6
	UserNotificationStateApp                  UserNotificationState = 7
)

// GetADTUserNotificationState is the Go port of Get-ADTUserNotificationState.
func GetADTUserNotificationState(ctx context.Context) (UserNotificationState, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("adt: %w", err)
	}
	v, err := queryUserNotificationState()
	if err != nil {
		return 0, err
	}
	return UserNotificationState(v), nil
}

// TestADTUserIsBusy is the Go port of Test-ADTUserIsBusy: it reports whether
// the user is in a state where notifications should be withheld.
func TestADTUserIsBusy(ctx context.Context) (bool, error) {
	state, err := GetADTUserNotificationState(ctx)
	if err != nil {
		return false, err
	}
	switch state {
	case UserNotificationStateAcceptsNotifications, UserNotificationStateApp:
		return false, nil
	default:
		return true, nil
	}
}

// TestADTUserInFocusMode is the Go port of Test-ADTUserInFocusMode: quiet time
// (Focus Assist) is reported as focus mode.
func TestADTUserInFocusMode(ctx context.Context) (bool, error) {
	state, err := GetADTUserNotificationState(ctx)
	if err != nil {
		return false, err
	}
	return state == UserNotificationStateQuietTime, nil
}
