package session

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/archive"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/deferral"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/logging"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/strtab"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/wts"
)

// Options mirrors the $adtSession hashtable of Invoke-AppDeployToolkit.ps1
// plus the Open-ADTSession parameters the Go port supports.
type Options struct {
	AppVendor        string
	AppName          string
	AppVersion       string
	AppArch          string
	AppLang          string
	AppRevision      string
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

	// Auto deploy-mode detection opt-outs (parity with Open-ADTSession).
	NoOobeDetection               bool // skip OOBE/ESP checks
	NoProcessDetection            bool // skip processes-to-close checks
	NoSessionDetection            bool // skip session-0 checks
	ProcessInteractivityDetection bool // in session 0, require an interactive station
}

// Deps injects the platform seams; zero values get live defaults on Windows
// and inert fakes elsewhere (see deps_windows.go / deps_other.go).
type Deps struct {
	Registry regkey.Backend
	WTS      wts.Query
	// IsAdmin reports whether the caller has administrative rights.
	IsAdmin func() bool
	// Culture returns the OS UI culture (e.g. "en-US"); may return "".
	Culture func() string
	// LogEcho receives every rendered log entry (console mirror). Optional.
	LogEcho func(e logging.Entry)
	// Now is the clock; defaults to time.Now.
	Now func() time.Time
	// OobeCompleted reports whether the Out-of-Box Experience is done.
	OobeCompleted func() (bool, error)
	// ProcessRunning reports whether any of the named processes is running.
	ProcessRunning func(names []string) (bool, error)
	// ProcessInteractive reports whether the process runs on an interactive
	// window station (WinSta0).
	ProcessInteractive func() bool
	// ActiveUserSID returns the SID of the active interactive user, or "".
	ActiveUserSID func() string
}

// Session is the Go port of PSADT's DeploymentSession.
type Session struct {
	mu sync.Mutex

	opts    Options
	deps    Deps
	cfg     *config.Config
	strings *strtab.Table
	env     *EnvironmentTable

	installName   string
	installTitle  string
	cultureSource string     // e.g. "[en-GB] via active user's display language"
	deployMode    DeployMode // resolved (never Auto)
	installPhase string
	exitCode     int
	closed       bool
	startTime    time.Time
	isAdmin      bool

	logWriter      *logging.Writer
	defaultLogName string // contains %s discriminator slot
	defers         *deferral.Store

	// compressLogDir is the temporary log-capture folder used when
	// Toolkit.CompressLogs is set; Close zips it into the configured LogPath.
	compressLogDir string
	// finalLogDir is the configured log destination (zip target when
	// compressing, the live log folder otherwise).
	finalLogDir string

	// ExitWithMsiCodes mirrors the zero-config MSI setting: on success the
	// session normalizes the exit code to 0/3010.
	ExitWithMsiCodes bool
}

var (
	invalidFileNameChars = regexp.MustCompile(`[\x00-\x1f<>:"/\\|?*]`)
	doubleUnderscore     = regexp.MustCompile(`_{2,}`)
	doubleSpace          = regexp.MustCompile(`\s{2,}`)
)

// Open validates the options, loads config and strings, resolves the deploy
// mode, initializes logging and deferral state, and writes the opening log
// entries. Port of DeploymentSession's constructor.
func Open(ctx context.Context, opts Options, deps Deps) (*Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("session: %w", err)
	}
	applyDepDefaults(&deps)

	s := &Session{opts: opts, deps: deps, startTime: deps.Now(), isAdmin: deps.IsAdmin()}

	if opts.RequireAdmin && !s.isAdmin {
		return nil, errs.Wrap(
			"session: RequireAdmin is set and the caller is not an administrator",
			errs.ErrInvalidOption,
		)
	}

	// Config + strings.
	cfg, err := config.Load(opts.ConfigOverlayPath)
	if err != nil {
		return nil, err
	}
	cfg.ExpandPaths()
	s.cfg = cfg

	// Language resolution: an explicit override wins; otherwise, when running
	// in session 0 (SYSTEM/service context), prefer the active interactive
	// user's Windows display language over the process culture (PSADT
	// Get-ADTStringLanguage parity).
	activeUserCulture := ""
	if sessID, err := deps.WTS.ProcessSessionID(); err == nil && sessID == 0 {
		activeUserCulture = userUICulture(deps.Registry, deps.ActiveUserSID())
	}
	culture, cultureSource := firstNonEmptyWithSource([][2]string{
		{opts.LanguageOverride, "session LanguageOverride"},
		{cfg.UI.LanguageOverride, "config UI.LanguageOverride"},
		{activeUserCulture, "active user's display language"},
		{deps.Culture(), "process UI culture"},
	})
	s.cultureSource = fmt.Sprintf("[%s] via %s", culture, cultureSource)
	tbl, err := strtab.Load(culture, opts.StringsOverlayPath)
	if err != nil {
		return nil, err
	}
	s.strings = tbl

	// Environment table.
	s.env = newEnvironmentTable(s.isAdmin)

	// Defaults mirroring DeploymentSession.
	if len(s.opts.AppSuccessExitCodes) == 0 {
		s.opts.AppSuccessExitCodes = []int{0}
	}
	if len(s.opts.AppRebootExitCodes) == 0 {
		s.opts.AppRebootExitCodes = []int{1641, 3010}
	}
	if s.opts.AppLang == "" {
		s.opts.AppLang = "EN"
	}
	if s.opts.AppRevision == "" {
		s.opts.AppRevision = "01"
	}
	if s.opts.AppName == "" {
		s.opts.AppName = s.env.AppDeployToolkitName
	}
	if s.opts.ScriptDirectory != "" {
		if s.opts.DirFiles == "" {
			s.opts.DirFiles = filepath.Join(s.opts.ScriptDirectory, "Files")
		}
		if s.opts.DirSupportFiles == "" {
			s.opts.DirSupportFiles = filepath.Join(s.opts.ScriptDirectory, "SupportFiles")
		}
	}

	s.composeNames()

	// Deferral store, keyed like PSADT: <RegPath>\PSAppDeployToolkit\DeferHistory\<InstallName>.
	regPath := cfg.Toolkit.RegPath
	if !s.isAdmin {
		regPath = cfg.Toolkit.RegPathNoAdminRights
	}
	store, err := deferral.NewStore(deps.Registry, regPath, s.installName)
	if err != nil {
		return nil, err
	}
	s.defers = store

	if err := s.initLogging(); err != nil {
		return nil, err
	}
	// Resolve after logging is up so each detection decision is logged live,
	// like DeploymentSession's SetDeploymentProperties.
	s.resolveDeployMode()
	s.writeOpeningEntries()
	return s, nil
}

func applyDepDefaults(d *Deps) {
	if d.Registry == nil {
		d.Registry = defaultRegistry()
	}
	if d.WTS == nil {
		d.WTS = defaultWTS()
	}
	if d.IsAdmin == nil {
		d.IsAdmin = defaultIsAdmin
	}
	if d.Culture == nil {
		d.Culture = defaultCulture
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.OobeCompleted == nil {
		d.OobeCompleted = defaultOobeCompleted
	}
	if d.ProcessRunning == nil {
		d.ProcessRunning = defaultProcessRunning
	}
	if d.ProcessInteractive == nil {
		d.ProcessInteractive = defaultProcessInteractive
	}
	if d.ActiveUserSID == nil {
		d.ActiveUserSID = defaultActiveUserSID
	}
}

// composeNames ports the InstallName/InstallTitle/DefaultLogName composition.
func (s *Session) composeNames() {
	o := &s.opts
	if strings.TrimSpace(o.InstallTitle) == "" {
		s.installTitle = strings.TrimSpace(
			fmt.Sprintf("%s %s %s", o.AppVendor, o.AppName, o.AppVersion),
		)
	} else {
		s.installTitle = o.InstallTitle
	}
	s.installTitle = doubleSpace.ReplaceAllString(s.installTitle, " ")

	name := o.InstallName
	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf(
			"%s_%s_%s_%s_%s_%s",
			o.AppVendor,
			o.AppName,
			o.AppVersion,
			o.AppArch,
			o.AppLang,
			o.AppRevision,
		)
	}
	name = strings.ReplaceAll(strings.Trim(name, "_"), " ", "")
	s.installName = invalidFileNameChars.ReplaceAllString(
		doubleUnderscore.ReplaceAllString(name, "_"),
		"",
	)

	userSuffix := ""
	if !s.isAdmin {
		userSuffix = "_" + s.env.EnvUserName
	}
	s.defaultLogName = invalidFileNameChars.ReplaceAllString(
		fmt.Sprintf("%s_%%s_%s%s.log", s.installName, o.DeploymentType, userSuffix), "")
}

// resolveDeployMode resolves the effective deploy mode. Explicit modes pass
// through; Auto runs the PSADT detection chain (see resolveAutoDeployMode).
func (s *Session) resolveDeployMode() {
	if s.opts.DeployMode != DeployModeAuto {
		s.deployMode = s.opts.DeployMode
		return
	}
	probes := deployModeProbes{
		oobeComplete: s.deps.OobeCompleted,
		espUserSetupActive: func() (bool, error) {
			running, err := s.deps.ProcessRunning([]string{"wwahost"})
			if err != nil || !running {
				return false, err
			}
			return espFirstSyncPending(s.deps.Registry, s.deps.ActiveUserSID())
		},
		sessionZero: func() bool {
			id, err := s.deps.WTS.ProcessSessionID()
			return err == nil && id == 0
		},
		processInteractive: s.deps.ProcessInteractive,
		activeUserPresent: func() bool {
			users, err := s.deps.WTS.LoggedOnUsers()
			if err != nil {
				return false
			}
			for _, u := range users {
				if u.IsActive {
					return true
				}
			}
			return false
		},
		processesToCloseRunning: func() (bool, error) {
			names := make([]string, len(s.opts.AppProcessesToClose))
			for i, p := range s.opts.AppProcessesToClose {
				names[i] = p.Name
			}
			return s.deps.ProcessRunning(names)
		},
	}
	s.deployMode = resolveAutoDeployMode(s.opts, probes, func(msg string) {
		s.WriteLog(msg, logging.SeverityInfo, "", "Initialization")
	})
}

func (s *Session) initLogging() error {
	if s.opts.DisableLogging {
		return nil
	}
	logDir := s.cfg.Toolkit.LogPath
	if !s.isAdmin {
		logDir = s.cfg.Toolkit.LogPathNoAdminRights
	}
	switch {
	case s.cfg.Toolkit.LogToHierarchy:
		logDir = filepath.Join(logDir, s.opts.AppVendor, s.opts.AppName, s.opts.AppVersion)
	case s.cfg.Toolkit.LogToSubfolder:
		logDir = filepath.Join(logDir, s.installName)
	}
	s.finalLogDir = logDir

	// CompressLogs parity (DeploymentSession.cs): write logs to a temp capture
	// folder during the session; Close zips them into the configured LogPath.
	if s.cfg.Toolkit.CompressLogs {
		tempPath := s.cfg.Toolkit.TempPath
		if !s.isAdmin {
			tempPath = s.cfg.Toolkit.TempPathNoAdminRights
		}
		capture := filepath.Join(
			tempPath,
			fmt.Sprintf("%s_%s", s.installName, s.opts.DeploymentType),
		)
		if err := os.RemoveAll(capture); err != nil {
			return fmt.Errorf("session: purging log capture folder %s: %w", capture, err)
		}
		s.compressLogDir = capture
		logDir = capture
	}
	logName := s.opts.LogName
	if strings.TrimSpace(logName) == "" {
		logName = s.NewLogFileName(s.env.AppDeployToolkitName)
	} else {
		logName = invalidFileNameChars.ReplaceAllString(logName, "")
	}
	w, err := logging.NewWriter(logging.Options{
		Directory:  logDir,
		FileName:   logName,
		Style:      logging.ParseStyle(s.cfg.Toolkit.LogStyle),
		Append:     s.cfg.Toolkit.LogAppend,
		MaxSizeMB:  s.cfg.Toolkit.LogMaxSize,
		MaxHistory: s.cfg.Toolkit.LogMaxHistory,
		Echo:       s.deps.LogEcho,
	})
	if err != nil {
		return err
	}
	s.logWriter = w
	return nil
}

func (s *Session) writeOpeningEntries() {
	s.WriteLog(logging.LogDivider, logging.SeverityInfo, "", "")
	s.WriteLog(
		fmt.Sprintf("[%s] %s started.", s.installName, s.opts.DeploymentType.Verb()),
		logging.SeverityInfo,
		"",
		"",
	)
	s.WriteLog(fmt.Sprintf(
		"[%s] session opened: mode [%s], title [%s], admin [%t].",
		s.env.AppDeployToolkitName,
		s.deployMode,
		s.installTitle,
		s.isAdmin,
	), logging.SeverityInfo, "", "")
	s.WriteLog(
		"The following locale was used to import UI messages: "+s.cultureSource+".",
		logging.SeverityInfo,
		"",
		"",
	)
}

// NewLogFileName ports DeploymentSession.NewLogFileName: the default log
// name with the discriminator substituted.
func (s *Session) NewLogFileName(discriminator string) string {
	return fmt.Sprintf(s.defaultLogName, discriminator)
}

// Accessors (property parity with DeploymentSession).

// InstallName returns the sanitized install name.
func (s *Session) InstallName() string { return s.installName }

// InstallTitle returns the display title.
func (s *Session) InstallTitle() string { return s.installTitle }

// DeploymentType returns the session's deployment verb.
func (s *Session) DeploymentType() DeploymentType { return s.opts.DeploymentType }

// DeployMode returns the resolved deploy mode (never Auto).
func (s *Session) DeployMode() DeployMode { return s.deployMode }

// IsSilent reports whether all UI must be suppressed.
func (s *Session) IsSilent() bool { return s.deployMode == DeployModeSilent }

// IsNonInteractive reports whether prompts must not block on user input.
func (s *Session) IsNonInteractive() bool { return s.deployMode == DeployModeNonInteractive }

// IsAdmin reports whether the session runs with administrative rights.
func (s *Session) IsAdmin() bool { return s.isAdmin }

// Config returns the resolved configuration.
func (s *Session) Config() *config.Config { return s.cfg }

// Strings returns the resolved string table.
func (s *Session) Strings() *strtab.Table { return s.strings }

// Environment returns the session environment table.
func (s *Session) Environment() *EnvironmentTable { return s.env }

// Options returns a copy of the (defaulted) session options.
func (s *Session) Options() Options { return s.opts }

// DirFiles returns the package Files directory.
func (s *Session) DirFiles() string { return s.opts.DirFiles }

// DirSupportFiles returns the package SupportFiles directory.
func (s *Session) DirSupportFiles() string { return s.opts.DirSupportFiles }

// LogPath returns the active log file path ("" when logging is disabled).
func (s *Session) LogPath() string {
	if s.logWriter == nil {
		return ""
	}
	return s.logWriter.Path()
}

// InstallPhase returns the current phase label used in log entries.
func (s *Session) InstallPhase() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.installPhase
}

// SetInstallPhase sets the phase label (mirrors $adtSession.InstallPhase).
func (s *Session) SetInstallPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.installPhase = phase
}

// ExitCode returns the session exit code.
func (s *Session) ExitCode() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exitCode
}

// SetExitCode sets the session exit code.
func (s *Session) SetExitCode(code int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.exitCode = code
}

// Closed reports whether Close has already run.
func (s *Session) Closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

// DeferHistory returns the persisted deferral state.
func (s *Session) DeferHistory() (deferral.History, error) {
	return s.defers.Get()
}

// SetDeferHistory persists the deferral state.
func (s *Session) SetDeferHistory(h deferral.History) error {
	return s.defers.Set(h)
}

// ResetDeferHistory removes the deferral state.
func (s *Session) ResetDeferHistory() error {
	return s.defers.Reset()
}

// WriteLog writes a toolkit log entry. Source defaults to the toolkit name;
// section defaults to the current install phase.
func (s *Session) WriteLog(message string, severity logging.Severity, source, section string) {
	if source == "" {
		source = s.env.AppDeployToolkitName
	}
	if section == "" {
		section = s.InstallPhase()
	}
	e := logging.Entry{
		Time:          s.deps.Now(),
		Message:       message,
		Severity:      severity,
		Source:        source,
		ScriptSection: section,
		Username:      s.env.EnvUserDomain + `\` + s.env.EnvUserName,
		ProcessID:     s.env.ProcessID,
		FileName:      source,
	}
	if s.logWriter != nil {
		_ = s.logWriter.Write(e) // logging failures never abort a deployment
	} else if s.deps.LogEcho != nil {
		s.deps.LogEcho(e)
	}
}

// GetDeploymentStatus ports DeploymentSession.GetDeploymentStatus.
func (s *Session) GetDeploymentStatus() Status {
	code := s.ExitCode()
	if code == s.cfg.UI.DefaultExitCode || code == s.cfg.UI.DeferExitCode {
		return StatusFastRetry
	}
	for _, c := range s.opts.AppRebootExitCodes {
		if code == c {
			return StatusRestartRequired
		}
	}
	for _, c := range s.opts.AppSuccessExitCodes {
		if code == c {
			return StatusComplete
		}
	}
	return StatusError
}

// Close ports DeploymentSession.Close: classifies the exit code, writes the
// closing log entries, resets deferral on success, and returns the final
// process exit code.
func (s *Session) Close(ctx context.Context) int {
	_ = ctx
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return s.exitCode
	}
	s.closed = true
	s.mu.Unlock()

	status := s.GetDeploymentStatus()
	elapsed := s.deps.Now().Sub(s.startTime).Seconds()
	deployString := func(result string) string {
		return fmt.Sprintf(
			"[%s] %s %s in [%.2f] seconds with exit code [%d].",
			s.installName,
			strings.ToLower(s.opts.DeploymentType.String()),
			result,
			elapsed,
			s.ExitCode(),
		)
	}

	switch status {
	case StatusFastRetry:
		s.WriteLog(deployString("was deferred"), logging.SeverityWarning, "", "")
	case StatusError:
		s.WriteLog(deployString("failed"), logging.SeverityError, "", "")
	case StatusRestartRequired, StatusComplete:
		if s.ExitWithMsiCodes {
			if status == StatusRestartRequired {
				s.SetExitCode(3010)
			} else {
				s.SetExitCode(0)
			}
		}
		s.WriteLog(deployString("completed"), logging.SeveritySuccess, "", "")
		if status == StatusRestartRequired && !s.opts.SuppressRebootPassThru {
			s.WriteLog("A restart has been flagged as required.", logging.SeverityWarning, "", "")
		} else {
			s.SetExitCode(0)
		}
		if err := s.ResetDeferHistory(); err != nil {
			s.WriteLog(
				fmt.Sprintf("Failed to reset deferral history: %v", err),
				logging.SeverityWarning,
				"",
				"",
			)
		}
	}

	s.WriteLog(logging.LogDivider, logging.SeverityInfo, "", "")
	if s.logWriter != nil {
		_ = s.logWriter.Close()
	}
	s.compressLogs()
	return s.ExitCode()
}

// compressLogs zips the temporary log-capture folder into the configured log
// path and prunes old archives to Toolkit.LogMaxHistory. Failures are logged
// as warnings via LogEcho (the writer is closed) and never change the exit
// code.
func (s *Session) compressLogs() {
	if s.compressLogDir == "" {
		return
	}
	warn := func(msg string) {
		if s.deps.LogEcho != nil {
			s.deps.LogEcho(logging.Entry{
				Time:     s.deps.Now(),
				Message:  msg,
				Severity: logging.SeverityWarning,
				Source:   "CloseADTSession",
			})
		}
	}
	if err := os.MkdirAll(s.finalLogDir, 0o755); err != nil {
		warn(fmt.Sprintf("Failed to create log archive folder: %v", err))
		return
	}
	prefix := fmt.Sprintf("%s_%s_", s.installName, s.opts.DeploymentType)
	dest := filepath.Join(s.finalLogDir, prefix+s.deps.Now().Format("20060102150405")+".zip")
	if err := archive.WriteZipArchive([]string{s.compressLogDir}, dest); err != nil {
		warn(fmt.Sprintf("Failed to compress logs to [%s]: %v", dest, err))
		return
	}
	if err := os.RemoveAll(s.compressLogDir); err != nil {
		warn(fmt.Sprintf("Failed to remove log capture folder: %v", err))
	}
	// Prune archives beyond LogMaxHistory (timestamped names sort oldest-first).
	archives, err := filepath.Glob(filepath.Join(s.finalLogDir, prefix+"*.zip"))
	if err != nil || s.cfg.Toolkit.LogMaxHistory <= 0 {
		return
	}
	sort.Strings(archives)
	for len(archives) > s.cfg.Toolkit.LogMaxHistory {
		if err := os.Remove(archives[0]); err != nil {
			warn(fmt.Sprintf("Failed to prune log archive [%s]: %v", archives[0], err))
			break
		}
		archives = archives[1:]
	}
}

// CompressLogDir returns the temporary log-capture folder when
// Toolkit.CompressLogs is active, or "" otherwise. MSI logging uses this so
// msiexec logs are captured in the final archive.
func (s *Session) CompressLogDir() string { return s.compressLogDir }

// IsCallerAdmin reports whether the current process has administrative
// rights (live check, independent of any open session).
func IsCallerAdmin() bool { return defaultIsAdmin() }
