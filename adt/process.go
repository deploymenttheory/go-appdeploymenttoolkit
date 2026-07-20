package adt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// errNotWindows is the sentinel returned by the non-Windows stubs of the
// Windows-only process functions.
var errNotWindows = winerr.ErrNotWindows

// ProcessResult mirrors PSADT's ProcessResult: the exit code and captured
// output streams of a completed process.
type ProcessResult struct {
	ExitCode int
	StdOut   string
	StdErr   string
}

// RunningProcess mirrors PSADT's RunningProcessInfo.
type RunningProcess = procmgmt.RunningProcess

// WindowInfo mirrors PSADT's WindowInfo.
type WindowInfo = procmgmt.WindowInfo

// msiBusyExitCode is ERROR_INSTALL_ALREADY_RUNNING, the exit code PSADT
// reports when the Windows Installer service is busy (msiexec 1618).
const msiBusyExitCode = 1618

// msiExecuteMutexName is the system-wide Windows Installer serialization
// mutex probed before msiexec launches.
const msiExecuteMutexName = `Global\_MSIExecute`

// StartADTProcessOptions mirrors the parameters of Start-ADTProcess.
type StartADTProcessOptions struct {
	// FilePath is the executable to launch; relative paths resolve against
	// the active session's DirFiles.
	FilePath string
	// ArgumentList is the raw argument string.
	ArgumentList string
	// WorkingDirectory overrides the session-derived default.
	WorkingDirectory string
	// WindowStyle is Normal (default, ""), Hidden, Minimized or Maximized.
	WindowStyle string
	// CreateNoWindow suppresses console window creation and enables
	// StdOut/StdErr capture, like -CreateNoWindow.
	CreateNoWindow bool
	// UseShellExecute launches without stream redirection.
	UseShellExecute bool
	// WaitForMsiExec gates the launch on the Global\_MSIExecute mutex
	// (implied when FilePath contains "msiexec").
	WaitForMsiExec bool
	// MsiExecWaitTime bounds the mutex wait; zero uses config
	// MSI.MutexWaitTime.
	MsiExecWaitTime time.Duration
	// SuccessExitCodes defaults to the session's AppSuccessExitCodes, [0]
	// sessionless. Specifying it disables session exit-code passback,
	// mirroring Start-ADTProcess.
	SuccessExitCodes []int
	// RebootExitCodes defaults to the session's AppRebootExitCodes,
	// [1641 3010] sessionless.
	RebootExitCodes []int
	// IgnoreExitCodes lists exit codes to ignore; "*" ignores every code.
	IgnoreExitCodes []string
	// PassThru is accepted for PSADT parity; the Go port always returns
	// the result object.
	PassThru bool
	// Timeout bounds the process runtime; zero waits indefinitely.
	Timeout time.Duration
	// NoWait returns immediately after launch with an empty result.
	NoWait bool
	// PriorityClass is Idle, BelowNormal, Normal (default, ""), AboveNormal,
	// High or RealTime.
	PriorityClass string
	// Verb launches through ShellExecuteEx with the given verb (e.g.
	// "runas"); stream capture is unavailable on this path.
	Verb string
	// SecureArgumentList keeps ArgumentList out of every log line.
	SecureArgumentList bool
	// StandardInput, when non-empty, is piped to the process's stdin.
	StandardInput string
	// StreamEncoding decodes captured StdOut/StdErr: "" (UTF-8),
	// "utf-16le", or "oem" (the console OEM code page; Windows only).
	StreamEncoding string
	// NoStreamLogging suppresses logging of captured StdOut/StdErr.
	NoStreamLogging bool
	// ExpandEnvironmentVariables expands %VAR% references in FilePath and
	// ArgumentList before launch.
	ExpandEnvironmentVariables bool
	// WaitForChildProcesses waits for the whole spawned process tree, not
	// just the root process.
	WaitForChildProcesses bool
	// KillChildProcessesWithParent terminates surviving children once the
	// root process exits.
	KillChildProcessesWithParent bool
	// NoTerminateOnTimeout leaves the process running when Timeout elapses
	// (the call still reports the timeout).
	NoTerminateOnTimeout bool
	// TimeoutAction is "Error" (default, "") or "Continue": Continue
	// returns without an error after a timeout.
	TimeoutAction string
	// ExitOnProcessFailure is accepted for PSADT parity. The Go contract
	// already provides it: a failure exit code returns an *ExitError that
	// Deployment.Run maps onto the session exit code and process exit.
	ExitOnProcessFailure bool
	// DenyUserTermination denies PROCESS_TERMINATE on the launched process
	// to the interactive user.
	DenyUserTermination bool
}

// StartADTProcessAsUserOptions mirrors the parameters of
// Start-ADTProcessAsUser: the Start-ADTProcess core plus target selection.
type StartADTProcessAsUserOptions struct {
	StartADTProcessOptions

	// UserName targets a specific logged-on user ("user" or `DOMAIN\user`);
	// empty targets the first active session's user.
	UserName string
	// AllUsers launches the process in every logged-on user session.
	AllUsers bool
	// UseLinkedAdminToken requires the user's linked (elevated) admin
	// token; the launch fails when the user has no split token. Mutually
	// exclusive with the other token switches.
	UseLinkedAdminToken bool
	// UseHighestAvailableToken uses the linked admin token when available,
	// else the base token.
	UseHighestAvailableToken bool
	// UseUnelevatedToken forces the limited token even for admin users.
	UseUnelevatedToken bool
	// InheritEnvironmentVariables passes the calling process's environment
	// instead of the target user's own block.
	InheritEnvironmentVariables bool
}

// StartADTProcess is the Go port of Start-ADTProcess: it launches a process
// (optionally gated on the MSI mutex), waits for it and classifies the exit
// code against the success/reboot lists. Success and reboot codes return a
// nil error (reboot codes additionally flag the session exit code); failure
// codes return an *ExitError carrying the code.
func StartADTProcess(ctx context.Context, opts StartADTProcessOptions) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTProcess: %w", err)
	}
	s, _ := GetADTSession() // nil session means sessionless operation
	processLog(fmt.Sprintf("Preparing to execute process [%s]...", opts.FilePath),
		LogSeverityInfo, "StartADTProcess")
	plan, err := buildProcessPlan(s, &opts)
	if err != nil {
		return nil, err
	}
	return runProcessPlan(ctx, s, plan, opts.WaitForMsiExec, opts.MsiExecWaitTime,
		func(ctx context.Context, lo procmgmt.LaunchOptions) (*procmgmt.LaunchResult, error) {
			return processLauncher.Launch(ctx, lo)
		})
}

// StartADTProcessAsUser is the Go port of Start-ADTProcessAsUser: it runs
// the process inside a logged-on user's session (Windows only; requires
// SYSTEM). Exit-code classification matches StartADTProcess.
func StartADTProcessAsUser(
	ctx context.Context,
	opts StartADTProcessAsUserOptions,
) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTProcessAsUser: %w", err)
	}
	return startADTProcessAsUser(ctx, opts)
}

// GetADTRunningProcesses is the Go port of Get-ADTRunningProcesses: it
// returns the running processes matching the given process objects
// (Windows only).
func GetADTRunningProcesses(ctx context.Context, procs []ProcessObject) ([]RunningProcess, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: GetADTRunningProcesses: %w", err)
	}
	if len(procs) == 0 {
		return nil, fmt.Errorf("adt: at least one process object is required: %w", ErrInvalidOption)
	}
	names := make([]string, 0, len(procs))
	specs := make([]procmgmt.ProcessSpec, 0, len(procs))
	for _, p := range procs {
		names = append(names, p.Name)
		specs = append(specs, procmgmt.ProcessSpec{Name: p.Name, Description: p.Description})
	}
	processLog(fmt.Sprintf("Checking for running processes: ['%s']", strings.Join(names, "', '")),
		LogSeverityInfo, "GetADTRunningProcesses")
	running, err := runningProcesses(specs)
	if err != nil {
		return nil, err
	}
	if len(running) == 0 {
		processLog("Specified processes are not running.", LogSeverityInfo, "GetADTRunningProcesses")
		return nil, nil
	}
	found := make([]string, 0, len(running))
	for _, p := range running {
		found = append(found, p.Name)
	}
	processLog(fmt.Sprintf("The following processes are running: ['%s'].", strings.Join(found, "', '")),
		LogSeverityInfo, "GetADTRunningProcesses")
	return running, nil
}

// TestADTMutexAvailabilityOptions mirrors the parameters of
// Test-ADTMutexAvailability.
type TestADTMutexAvailabilityOptions struct {
	// MutexName is the system mutex to probe, e.g. `Global\_MSIExecute`.
	MutexName string
	// MutexWaitTime bounds the acquisition wait; zero uses PSADT's default
	// of one millisecond, negative waits indefinitely.
	MutexWaitTime time.Duration
}

// TestADTMutexAvailability is the Go port of Test-ADTMutexAvailability: it
// reports whether an exclusive lock on the named system mutex could be
// acquired within the wait time. Unexpected probe failures err on the side
// of availability, matching PSADT.
func TestADTMutexAvailability(ctx context.Context, opts TestADTMutexAvailabilityOptions) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: TestADTMutexAvailability: %w", err)
	}
	if opts.MutexName == "" {
		return false, fmt.Errorf("adt: MutexName is required: %w", ErrInvalidOption)
	}
	wait := opts.MutexWaitTime
	if wait == 0 {
		wait = time.Millisecond // Test-ADTMutexAvailability default
	}
	processLog(fmt.Sprintf("Checking to see if mutex [%s] is available. Wait up to [%s] for the mutex to become available.",
		opts.MutexName, wait), LogSeverityInfo, "TestADTMutexAvailability")
	free, err := mutexAvailable(opts.MutexName, wait)
	if err != nil {
		if isNotWindowsErr(err) {
			return false, err
		}
		// PSADT defaults to "available" when the check itself fails.
		processLog(fmt.Sprintf("Unable to check if mutex [%s] is available: %v. Will default to return value of [true].",
			opts.MutexName, err), LogSeverityError, "TestADTMutexAvailability")
		return true, nil
	}
	if free {
		processLog(fmt.Sprintf("Mutex [%s] is available for an exclusive lock.", opts.MutexName),
			LogSeverityInfo, "TestADTMutexAvailability")
	} else {
		processLog(fmt.Sprintf("Mutex [%s] is not available because another thread already has an exclusive lock on it.",
			opts.MutexName), LogSeverityInfo, "TestADTMutexAvailability")
	}
	return free, nil
}

// GetADTWindowTitleOptions mirrors the parameters of Get-ADTWindowTitle.
type GetADTWindowTitleOptions struct {
	// WindowTitle is a case-insensitive substring to match against open
	// window titles.
	WindowTitle string
	// GetAllWindowTitles returns every visible titled window instead.
	GetAllWindowTitles bool
}

// GetADTWindowTitle is the Go port of Get-ADTWindowTitle: it returns the
// visible top-level windows matching the options (Windows only).
func GetADTWindowTitle(ctx context.Context, opts GetADTWindowTitleOptions) ([]WindowInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: GetADTWindowTitle: %w", err)
	}
	if !opts.GetAllWindowTitles && opts.WindowTitle == "" {
		return nil, fmt.Errorf(
			"adt: WindowTitle or GetAllWindowTitles is required: %w", ErrInvalidOption)
	}
	if opts.GetAllWindowTitles {
		processLog("Finding all open window title(s).", LogSeverityInfo, "GetADTWindowTitle")
	} else {
		processLog("Finding open windows matching the specified criteria.",
			LogSeverityInfo, "GetADTWindowTitle")
	}
	windows, err := windowTitles()
	if err != nil {
		return nil, err
	}
	if opts.GetAllWindowTitles {
		return windows, nil
	}
	needle := strings.ToLower(opts.WindowTitle)
	matches := make([]WindowInfo, 0, len(windows))
	for _, w := range windows {
		if strings.Contains(strings.ToLower(w.Title), needle) {
			matches = append(matches, w)
		}
	}
	return matches, nil
}

// GetADTPEFileArchitecture is the Go port of Get-ADTPEFileArchitecture: it
// returns the PE image's architecture ("x86", "x64", "ARM64", ...).
func GetADTPEFileArchitecture(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: GetADTPEFileArchitecture: %w", err)
	}
	arch, err := procmgmt.PEFileArchitecture(path)
	if err != nil {
		return "", err
	}
	processLog(fmt.Sprintf("File [%s] has a detected file architecture of [%s].", path, arch),
		LogSeverityInfo, "GetADTPEFileArchitecture")
	return arch, nil
}

// InvokeADTCommandWithRetriesOptions mirrors the parameters of
// Invoke-ADTCommandWithRetries.
type InvokeADTCommandWithRetriesOptions struct {
	// Attempts is the number of retries after a failed first invocation
	// (default 3, so the function runs at most Attempts+1 times).
	Attempts int
	// DelaySeconds is the wait between attempts (default 5).
	DelaySeconds int
}

// InvokeADTCommandWithRetries is the Go port of Invoke-ADTCommandWithRetries:
// it invokes fn, retrying on any error with the configured delay, and
// returns the last error once attempts are exhausted.
func InvokeADTCommandWithRetries(
	ctx context.Context,
	opts InvokeADTCommandWithRetriesOptions,
	fn func(context.Context) error,
) error {
	if fn == nil {
		return fmt.Errorf("adt: a command function is required: %w", ErrInvalidOption)
	}
	logged := func(ctx context.Context) error {
		err := fn(ctx)
		if err != nil {
			processLog(fmt.Sprintf("The invocation failed with message: %v.", err),
				LogSeverityWarning, "InvokeADTCommandWithRetries")
		}
		return err
	}
	return procmgmt.Retry(ctx, opts.Attempts, time.Duration(opts.DelaySeconds)*time.Second, logged)
}

// processPlan is the resolved launch strategy for one Start-ADTProcess
// invocation.
type processPlan struct {
	launch          procmgmt.LaunchOptions
	success         []int
	reboot          []int
	ignore          []string
	canSetExit      bool // session exit-code passback allowed (default code lists)
	filePath        string
	secureArgs      bool
	streamEncoding  string
	noStreamLogging bool
	timeoutContinue bool // TimeoutAction "Continue"
}

// launchFunc abstracts "launch and wait" so runProcessPlan serves both the
// own-session and the as-user paths.
type launchFunc func(ctx context.Context, opts procmgmt.LaunchOptions) (*procmgmt.LaunchResult, error)

// buildProcessPlan resolves paths, defaults and exit-code lists the way
// Start-ADTProcess's begin block does.
func buildProcessPlan(s *DeploymentSession, opts *StartADTProcessOptions) (*processPlan, error) {
	style, ok := procmgmt.ParseWindowStyle(opts.WindowStyle)
	if !ok {
		return nil, fmt.Errorf("adt: WindowStyle %q: %w", opts.WindowStyle, ErrInvalidOption)
	}
	priority, ok := procmgmt.ParsePriorityClass(opts.PriorityClass)
	if !ok {
		return nil, fmt.Errorf("adt: PriorityClass %q: %w", opts.PriorityClass, ErrInvalidOption)
	}
	timeoutContinue, ok := procmgmt.ParseTimeoutAction(opts.TimeoutAction)
	if !ok {
		return nil, fmt.Errorf("adt: TimeoutAction %q: %w", opts.TimeoutAction, ErrInvalidOption)
	}
	if !procmgmt.ValidStreamEncoding(opts.StreamEncoding) {
		return nil, fmt.Errorf("adt: StreamEncoding %q: %w", opts.StreamEncoding, ErrInvalidOption)
	}
	if opts.ExpandEnvironmentVariables {
		opts.FilePath = expandWindowsEnv(opts.FilePath)
		opts.ArgumentList = expandWindowsEnv(opts.ArgumentList)
	}
	filePath := resolveProcessFilePath(s, opts.FilePath)
	workDir := opts.WorkingDirectory
	if workDir == "" && s != nil {
		// Session default: the executable's own directory for fully
		// qualified non-msiexec paths, else the package Files directory.
		if filepath.IsAbs(filePath) && filepath.Ext(filePath) != "" && !isMsiExec(filePath) {
			workDir = filepath.Dir(filePath)
		} else if s.DirFiles() != "" {
			workDir = s.DirFiles()
		}
	}
	success, reboot := opts.SuccessExitCodes, opts.RebootExitCodes
	canSetExit := success == nil && reboot == nil
	if success == nil {
		if s != nil {
			success = s.Options().AppSuccessExitCodes
		} else {
			success = []int{0}
		}
	}
	if reboot == nil {
		if s != nil {
			reboot = s.Options().AppRebootExitCodes
		} else {
			reboot = []int{ExitCodeHardReboot, ExitCodeRebootRequired}
		}
	}
	return &processPlan{
		launch: procmgmt.LaunchOptions{
			FilePath:                     filePath,
			ArgumentList:                 opts.ArgumentList,
			WorkingDirectory:             workDir,
			WindowStyle:                  style,
			CreateNoWindow:               opts.CreateNoWindow,
			UseShellExecute:              opts.UseShellExecute || opts.Verb != "",
			WaitForMsiExec:               opts.WaitForMsiExec,
			MsiExecWaitTime:              opts.MsiExecWaitTime,
			Timeout:                      opts.Timeout,
			NoWait:                       opts.NoWait,
			PriorityClass:                priority,
			StandardInput:                opts.StandardInput,
			NoTerminateOnTimeout:         opts.NoTerminateOnTimeout,
			WaitForChildProcesses:        opts.WaitForChildProcesses,
			KillChildProcessesWithParent: opts.KillChildProcessesWithParent,
			Verb:                         opts.Verb,
			DenyUserTermination:          opts.DenyUserTermination,
		},
		success:         success,
		reboot:          reboot,
		ignore:          opts.IgnoreExitCodes,
		canSetExit:      canSetExit,
		filePath:        filePath,
		secureArgs:      opts.SecureArgumentList,
		streamEncoding:  opts.StreamEncoding,
		noStreamLogging: opts.NoStreamLogging,
		timeoutContinue: timeoutContinue,
	}, nil
}

// runProcessPlan performs the MSI mutex gate, the launch and the exit-code
// classification shared by StartADTProcess and StartADTProcessAsUser.
func runProcessPlan(
	ctx context.Context,
	s *DeploymentSession,
	plan *processPlan,
	waitForMsiExec bool,
	msiWait time.Duration,
	launch launchFunc,
) (*ProcessResult, error) {
	if waitForMsiExec || isMsiExec(plan.filePath) {
		free, err := msiMutexAvailable(s, msiWait)
		if err != nil && !isNotWindowsErr(err) {
			processLog(fmt.Sprintf("Unable to check the MSI mutex: %v. Proceeding as available.", err),
				LogSeverityWarning, "StartADTProcess")
		}
		if !free {
			processLog("Another MSI installation is in progress; the Windows Installer service is unavailable (1618).",
				LogSeverityError, "StartADTProcess")
			return &ProcessResult{ExitCode: msiBusyExitCode},
				NewExitError(msiBusyExitCode, ErrTimeout)
		}
	}

	processLog(executingMessage(plan), LogSeverityInfo, "StartADTProcess")
	res, err := launch(ctx, plan.launch)
	if err != nil {
		if errors.Is(err, ErrTimeout) && plan.timeoutContinue {
			processLog("The process wait timed out; continuing without error per TimeoutAction [Continue].",
				LogSeverityWarning, "StartADTProcess")
			return &ProcessResult{ExitCode: msiBusyExitCode}, nil
		}
		processLog(fmt.Sprintf("Error occurred while attempting to start the specified process: %v", err),
			LogSeverityError, "StartADTProcess")
		return nil, err
	}
	if plan.launch.NoWait {
		return &ProcessResult{}, nil
	}
	return classifyProcessResult(s, plan, res)
}

// classifyProcessResult ports Start-ADTProcess's exit-code handling: ignored
// and successful codes return cleanly, reboot codes flag the session, and
// anything else surfaces as an *ExitError.
func classifyProcessResult(
	s *DeploymentSession,
	plan *processPlan,
	res *procmgmt.LaunchResult,
) (*ProcessResult, error) {
	result := &ProcessResult{
		ExitCode: res.ExitCode,
		StdOut:   decodeStream(res.StdOut, plan.streamEncoding),
		StdErr:   decodeStream(res.StdErr, plan.streamEncoding),
	}
	code := res.ExitCode
	if !plan.noStreamLogging {
		if out := strings.TrimSpace(result.StdOut); out != "" {
			processLog("Process StdOut:\n"+out, LogSeverityInfo, "StartADTProcess")
		}
		if errOut := strings.TrimSpace(result.StdErr); errOut != "" {
			processLog("Process StdErr:\n"+errOut, LogSeverityWarning, "StartADTProcess")
		}
	}
	switch {
	case exitCodeIgnored(plan.ignore, code):
		processLog(fmt.Sprintf("Execution completed and the exit code [%d] is being ignored.", code),
			LogSeverityInfo, "StartADTProcess")
		return result, nil
	case containsExitCode(plan.success, code):
		processLog(fmt.Sprintf("Execution completed successfully with exit code [%d].", code),
			LogSeveritySuccess, "StartADTProcess")
		setSessionExitCode(s, plan, code)
		return result, nil
	case containsExitCode(plan.reboot, code):
		processLog(fmt.Sprintf("Execution completed successfully with exit code [%d]. A reboot is required.", code),
			LogSeverityWarning, "StartADTProcess")
		setSessionExitCode(s, plan, code)
		return result, nil
	default:
		processLog(fmt.Sprintf("Execution failed with exit code [%d].", code),
			LogSeverityError, "StartADTProcess")
		setSessionExitCode(s, plan, code)
		return result, NewExitError(code, nil)
	}
}

// setSessionExitCode ports Start-ADTProcess's Set-ADTSessionExitCode worker:
// the process exit code is passed back to the session only when the default
// code lists are in play, and never downgrades a more severe status.
func setSessionExitCode(s *DeploymentSession, plan *processPlan, code int) {
	if s == nil || !plan.canSetExit {
		return
	}
	status := s.GetDeploymentStatus()
	switch {
	case containsExitCode(plan.success, code):
		if status <= session.StatusComplete {
			s.SetExitCode(code)
		}
	case containsExitCode(plan.reboot, code):
		if status <= session.StatusRestartRequired {
			s.SetExitCode(code)
		}
	default:
		s.SetExitCode(code) // failure codes always stick
	}
}

// msiMutexAvailable waits on Global\_MSIExecute, defaulting the wait to the
// config MSI.MutexWaitTime (session config when active, embedded default
// otherwise).
func msiMutexAvailable(s *DeploymentSession, wait time.Duration) (bool, error) {
	if wait <= 0 {
		seconds := 600 // embedded config default, last-resort fallback
		if s != nil {
			seconds = s.Config().MSI.MutexWaitTime
		} else if cfg, err := config.Default(); err == nil {
			seconds = cfg.MSI.MutexWaitTime
		}
		wait = time.Duration(seconds) * time.Second
	}
	return mutexAvailable(msiExecuteMutexName, wait)
}

// resolveProcessFilePath resolves a relative FilePath against the active
// session's DirFiles, mirroring Start-ADTProcess's Resolve-ADTFileSystemPath
// behavior for packaged payloads.
func resolveProcessFilePath(s *DeploymentSession, filePath string) string {
	if filePath == "" || filepath.IsAbs(filePath) || s == nil || s.DirFiles() == "" {
		return filePath
	}
	candidate := filepath.Join(s.DirFiles(), filePath)
	if _, err := os.Stat(candidate); err != nil {
		return filePath
	}
	processLog(fmt.Sprintf("File path [%s] successfully resolved to fully qualified path [%s].",
		filePath, candidate), LogSeverityInfo, "StartADTProcess")
	return candidate
}

// executingMessage formats the pre-launch log line. SecureArgumentList keeps
// the composed arguments out of the log.
func executingMessage(plan *processPlan) string {
	suffix := ""
	if plan.launch.NoWait {
		suffix = " without waiting"
	}
	if plan.launch.ArgumentList != "" {
		if plan.secureArgs {
			return fmt.Sprintf("Executing [\"%s\"] with secure arguments%s...", plan.filePath, suffix)
		}
		return fmt.Sprintf("Executing [\"%s\" %s]%s...", plan.filePath, plan.launch.ArgumentList, suffix)
	}
	return fmt.Sprintf("Executing [\"%s\"]%s...", plan.filePath, suffix)
}

// decodeStream converts a captured raw stream per the StreamEncoding option:
// "" passes through (UTF-8), "utf-16le" decodes UTF-16 little-endian, "oem"
// decodes the console OEM code page (Windows only; passthrough elsewhere).
func decodeStream(s, encoding string) string {
	switch strings.ToLower(encoding) {
	case "utf-16le":
		return decodeUTF16LE(s)
	case "oem":
		return decodeOEM(s)
	default:
		return s
	}
}

// decodeUTF16LE decodes a raw byte string of UTF-16 little-endian text,
// dropping a leading BOM.
func decodeUTF16LE(s string) string {
	b := []byte(s)
	if len(b) < 2 {
		return s
	}
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u16 = append(u16, uint16(b[i])|uint16(b[i+1])<<8)
	}
	if len(u16) > 0 && u16[0] == 0xFEFF {
		u16 = u16[1:]
	}
	return string(utf16.Decode(u16))
}

// expandWindowsEnv expands %VAR% references from the process environment.
func expandWindowsEnv(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var out strings.Builder
	for {
		start := strings.IndexByte(s, '%')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start+1:], '%')
		if end < 0 {
			break
		}
		name := s[start+1 : start+1+end]
		out.WriteString(s[:start])
		if val, ok := os.LookupEnv(name); ok && name != "" {
			out.WriteString(val)
		} else {
			out.WriteString("%" + name + "%") // unknown vars stay verbatim
		}
		s = s[start+2+end:]
	}
	out.WriteString(s)
	return out.String()
}

// isMsiExec reports whether the target is the Windows Installer executable.
func isMsiExec(filePath string) bool {
	return strings.Contains(strings.ToLower(filePath), "msiexec")
}

// exitCodeIgnored reports whether the code matches the -IgnoreExitCodes
// list, honoring the "*" wildcard.
func exitCodeIgnored(ignore []string, code int) bool {
	for _, entry := range ignore {
		if entry == "*" {
			return true
		}
		if n, err := strconv.Atoi(entry); err == nil && n == code {
			return true
		}
	}
	return false
}

// containsExitCode reports list membership.
func containsExitCode(codes []int, code int) bool {
	for _, c := range codes {
		if c == code {
			return true
		}
	}
	return false
}

// isNotWindowsErr reports the non-Windows stub sentinel without importing
// winerr at every call site.
func isNotWindowsErr(err error) bool {
	return errors.Is(err, errNotWindows)
}

// processLog writes to the active session's log when one is open; process
// functions remain fully usable sessionless.
func processLog(message string, severity LogSeverity, source string) {
	if s, err := GetADTSession(); err == nil {
		s.WriteLog(message, severity, source, "")
	}
}
