package deploy

import (
	"context"
	"sync"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/session"
)

// Session is the deployment session: configuration, strings, environment,
// logging, deferral state and exit-code semantics for one deployment run.
type Session = session.Session

// SessionOptions mirrors the session metadata a deployment declares (the
// $adtSession hashtable of PSADT's frontend, platform-neutralized).
type SessionOptions struct {
	AppVendor        string
	AppName          string
	AppVersion       string
	AppArch          string
	AppLang          string // default "EN"
	AppRevision      string // default "01"
	AppScriptVersion string
	AppScriptDate    string
	AppScriptAuthor  string

	InstallName  string // default composed from app metadata
	InstallTitle string // default "<vendor> <name> <version>"

	DeploymentType DeploymentType
	DeployMode     DeployMode

	AppProcessesToClose []ProcessObject
	AppSuccessExitCodes []int // default [0]
	AppRebootExitCodes  []int // default [1641, 3010]

	ScriptDirectory string // package root; other Dir* default beneath it
	DirFiles        string // default <ScriptDirectory>/Files
	DirSupportFiles string // default <ScriptDirectory>/SupportFiles

	LogName                string
	DisableLogging         bool
	SuppressRebootPassThru bool
	TerminalServerMode     bool
	RequireAdmin           bool

	ConfigOverlayPath  string // package Config/config.yaml
	StringsOverlayPath string // package Strings/strings.yaml
	LanguageOverride   string // wins over config UI.LanguageOverride

	// Auto deploy-mode detection opt-outs. With DeployMode Auto the session
	// resolves NonInteractive during OOBE/ESP, Silent in session 0 without
	// an interactive station, and Silent when no AppProcessesToClose entry
	// is running (or none is specified). The probes are platform-seamed;
	// on platforms without an implementation they are inert.
	NoOobeDetection               bool
	NoProcessDetection            bool
	NoSessionDetection            bool
	ProcessInteractivityDetection bool

	// Hooks carries the module callbacks invoked around session open/close,
	// per log entry and on deferral.
	Hooks Hooks
}

// internalOptions maps the public options onto the session engine's options.
func (o SessionOptions) internalOptions() session.Options {
	return session.Options{
		AppVendor:              o.AppVendor,
		AppName:                o.AppName,
		AppVersion:             o.AppVersion,
		AppArch:                o.AppArch,
		AppLang:                o.AppLang,
		AppRevision:            o.AppRevision,
		AppScriptVersion:       o.AppScriptVersion,
		AppScriptDate:          o.AppScriptDate,
		AppScriptAuthor:        o.AppScriptAuthor,
		InstallName:            o.InstallName,
		InstallTitle:           o.InstallTitle,
		DeploymentType:         o.DeploymentType,
		DeployMode:             o.DeployMode,
		AppProcessesToClose:    o.AppProcessesToClose,
		AppSuccessExitCodes:    o.AppSuccessExitCodes,
		AppRebootExitCodes:     o.AppRebootExitCodes,
		ScriptDirectory:        o.ScriptDirectory,
		DirFiles:               o.DirFiles,
		DirSupportFiles:        o.DirSupportFiles,
		LogName:                o.LogName,
		DisableLogging:         o.DisableLogging,
		SuppressRebootPassThru: o.SuppressRebootPassThru,
		TerminalServerMode:     o.TerminalServerMode,
		RequireAdmin:           o.RequireAdmin,
		ConfigOverlayPath:      o.ConfigOverlayPath,
		StringsOverlayPath:     o.StringsOverlayPath,
		LanguageOverride:       o.LanguageOverride,

		NoOobeDetection:               o.NoOobeDetection,
		NoProcessDetection:            o.NoProcessDetection,
		NoSessionDetection:            o.NoSessionDetection,
		ProcessInteractivityDetection: o.ProcessInteractivityDetection,
	}
}

// Hooks carries the module callback stages: Starting runs before the
// session opens, Opening right after it opens, Closing before it closes and
// Finishing after it has closed. OnLogEntry receives every rendered log
// entry, and OnDefer runs when a welcome prompt resolves to a deferral.
type Hooks struct {
	Starting  []func(ctx context.Context) error
	Opening   []func(ctx context.Context, s *Session) error
	Closing   []func(ctx context.Context, s *Session) error
	Finishing []func(ctx context.Context) error
	// OnLogEntry must not call session logging APIs (it would re-enter).
	OnLogEntry []func(e LogEntry)
	OnDefer    []func(ctx context.Context, s *Session)
}

// sessionStack tracks open sessions like PSADT's module session stack.
var sessionStack struct {
	mu       sync.Mutex
	sessions []*Session
	hooks    map[*Session]Hooks
}

// Open instantiates a deployment session (config, strings, environment
// table, log file, deferral state, resolved deploy mode) and pushes it onto
// the active-session stack.
func Open(ctx context.Context, opts SessionOptions) (*Session, error) {
	for _, hook := range opts.Hooks.Starting {
		if err := hook(ctx); err != nil {
			return nil, err
		}
	}
	deps := session.Deps{}
	if entryHooks := opts.Hooks.OnLogEntry; len(entryHooks) > 0 {
		deps.LogEcho = func(e LogEntry) {
			for _, h := range entryHooks {
				h(e)
			}
		}
	}
	s, err := session.Open(ctx, opts.internalOptions(), deps)
	if err != nil {
		return nil, err
	}
	sessionStack.mu.Lock()
	sessionStack.sessions = append(sessionStack.sessions, s)
	if sessionStack.hooks == nil {
		sessionStack.hooks = map[*Session]Hooks{}
	}
	sessionStack.hooks[s] = opts.Hooks
	sessionStack.mu.Unlock()
	for _, hook := range opts.Hooks.Opening {
		if err := hook(ctx, s); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Current returns the most recently opened active session.
func Current() (*Session, error) {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	if len(sessionStack.sessions) == 0 {
		return nil, ErrNoActiveSession
	}
	return sessionStack.sessions[len(sessionStack.sessions)-1], nil
}

// Active reports whether a session is open.
func Active() bool {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	return len(sessionStack.sessions) > 0
}

// Close finalizes a session: logging, exit-code classification (honoring
// success/reboot code lists and reboot-passthru suppression), deferral reset
// on success, stack removal; returns the final process exit code.
func Close(ctx context.Context, s *Session) int {
	sessionStack.mu.Lock()
	hooks := sessionStack.hooks[s]
	sessionStack.mu.Unlock()

	for _, hook := range hooks.Closing {
		if err := hook(ctx, s); err != nil {
			s.WriteLog("Closing hook failed: "+err.Error(), 2, "CloseSession", "")
		}
	}
	code := s.Close(ctx)
	sessionStack.mu.Lock()
	for i, open := range sessionStack.sessions {
		if open == s {
			sessionStack.sessions = append(sessionStack.sessions[:i], sessionStack.sessions[i+1:]...)
			break
		}
	}
	delete(sessionStack.hooks, s)
	sessionStack.mu.Unlock()
	for _, hook := range hooks.Finishing {
		if err := hook(ctx); err != nil {
			code = max(code, 0) // finishing hook failures never change the exit code
		}
	}
	return code
}

// CallerIsAdmin is a live check of whether the process has administrative
// rights, independent of any open session.
func CallerIsAdmin() bool {
	return session.IsCallerAdmin()
}

// HooksOf returns the hooks registered for an open session (zero value
// after close or for an unknown session). Platform SDKs use it to dispatch
// OnDefer from their dialog implementations.
func HooksOf(s *Session) Hooks {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	return sessionStack.hooks[s]
}

// AddClosingHook appends a Closing hook to an already-open session, so
// features enabled mid-deployment (e.g. app-execution blocking) can register
// their cleanup to run at Close.
func AddClosingHook(s *Session, fn func(ctx context.Context, s *Session) error) {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	if sessionStack.hooks == nil {
		sessionStack.hooks = map[*Session]Hooks{}
	}
	h := sessionStack.hooks[s]
	h.Closing = append(h.Closing, fn)
	sessionStack.hooks[s] = h
}
