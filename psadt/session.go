package psadt

import (
	"context"
	"sync"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
)

// DeploymentSession is the Go port of PSADT's DeploymentSession object.
type DeploymentSession = session.Session

// SessionOptions mirrors the $adtSession hashtable passed to Open-ADTSession.
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

	// Hooks is the Go port of the Add-ADTModuleCallback families: functions
	// invoked around session open and close.
	Hooks SessionHooks
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
	}
}

// SessionHooks carries the module callback stages: Starting runs before the
// session opens, Opening right after it opens, Closing before it closes and
// Finishing after it has closed.
type SessionHooks struct {
	Starting  []func(ctx context.Context) error
	Opening   []func(ctx context.Context, s *DeploymentSession) error
	Closing   []func(ctx context.Context, s *DeploymentSession) error
	Finishing []func(ctx context.Context) error
}

// sessionStack tracks open sessions like PSADT's module session stack.
var sessionStack struct {
	mu       sync.Mutex
	sessions []*DeploymentSession
	hooks    map[*DeploymentSession]SessionHooks
}

// OpenADTSession is the Go port of Open-ADTSession: it instantiates a
// deployment session (config, strings, environment table, log file, deferral
// state, resolved deploy mode) and pushes it onto the active-session stack.
func OpenADTSession(ctx context.Context, opts SessionOptions) (*DeploymentSession, error) {
	for _, hook := range opts.Hooks.Starting {
		if err := hook(ctx); err != nil {
			return nil, err
		}
	}
	s, err := session.Open(ctx, opts.internalOptions(), session.Deps{})
	if err != nil {
		return nil, err
	}
	sessionStack.mu.Lock()
	sessionStack.sessions = append(sessionStack.sessions, s)
	if sessionStack.hooks == nil {
		sessionStack.hooks = map[*DeploymentSession]SessionHooks{}
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

// GetADTSession is the Go port of Get-ADTSession: it returns the most
// recently opened active session.
func GetADTSession() (*DeploymentSession, error) {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	if len(sessionStack.sessions) == 0 {
		return nil, ErrNoActiveSession
	}
	return sessionStack.sessions[len(sessionStack.sessions)-1], nil
}

// TestADTSessionActive is the Go port of Test-ADTSessionActive.
func TestADTSessionActive() bool {
	sessionStack.mu.Lock()
	defer sessionStack.mu.Unlock()
	return len(sessionStack.sessions) > 0
}

// CloseADTSession is the Go port of Close-ADTSession: it finalizes logging,
// classifies the exit code (honoring AppSuccessExitCodes/AppRebootExitCodes
// and reboot-passthru suppression), resets deferral history on success, pops
// the session from the stack and returns the final process exit code.
func CloseADTSession(ctx context.Context, s *DeploymentSession) int {
	sessionStack.mu.Lock()
	hooks := sessionStack.hooks[s]
	sessionStack.mu.Unlock()

	for _, hook := range hooks.Closing {
		if err := hook(ctx, s); err != nil {
			s.WriteLog("Closing hook failed: "+err.Error(), 2, "CloseADTSession", "")
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

// TestADTCallerIsAdmin is the Go port of Test-ADTCallerIsAdmin: a live check
// of whether the process has administrative rights.
func TestADTCallerIsAdmin() bool {
	return session.IsCallerAdmin()
}
