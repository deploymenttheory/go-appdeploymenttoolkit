package adt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/msipkg"
)

// InstalledApplication re-exports the msipkg application record returned by
// GetADTApplication (PSADT's PSADT.AppManagement.InstalledApplication).
type InstalledApplication = msipkg.InstalledApplication

// StartADTMsiProcessOptions mirrors the parameters of Start-ADTMsiProcess.
type StartADTMsiProcessOptions struct {
	// Action is Install (default, ""), Uninstall, Repair, Patch or
	// ActiveSetup.
	Action string
	// Path is the .msi/.msp file (relative paths resolve against the active
	// session's DirFiles) or an MSI product code GUID.
	Path string
	// Transforms lists MST files applied via TRANSFORMS=; relative names
	// resolve against the MSI's directory.
	Transforms []string
	// ArgumentList overrides the default MSI parameters from config.
	ArgumentList string
	// AdditionalArgumentList appends to the default (or overridden) MSI
	// parameters.
	AdditionalArgumentList string
	// LoggingOptions overrides config MSI.LoggingOptions (default "/L*V").
	LoggingOptions string
	// LogFileName overrides the log file name derived from the MSI.
	LogFileName string
	// SecureArgumentList suppresses this facade's argument logging.
	//
	// Deviation from PSADT: the underlying StartADTProcess still logs its
	// own "Executing [...]" line including the composed arguments.
	SecureArgumentList bool
	// SkipMSIAlreadyInstalledCheck skips the installed-product check.
	SkipMSIAlreadyInstalledCheck bool
	// SuccessExitCodes overrides the exit codes considered successful
	// (nil uses the session defaults plus the MSI success set).
	SuccessExitCodes []int
	// RebootExitCodes overrides the exit codes flagging a reboot.
	RebootExitCodes []int
	// PassThru is accepted for PSADT parity; the Go port always returns the
	// result object.
	PassThru bool
}

// StartADTMsiProcessAsUserOptions mirrors Start-ADTMsiProcessAsUser: the
// Start-ADTMsiProcess core plus target-user selection.
type StartADTMsiProcessAsUserOptions struct {
	StartADTMsiProcessOptions

	// UserName targets a specific logged-on user; empty targets the first
	// active session's user.
	UserName string
	// AllUsers runs the MSI operation in every logged-on user session.
	AllUsers bool
}

// StartADTMsiProcess is the Go port of Start-ADTMsiProcess: it composes an
// msiexec.exe command line from the action, package/product code, transforms,
// config-driven default parameters and verbose logging options, and delegates
// to StartADTProcess (which serializes on the Global\_MSIExecute mutex).
//
// As in PSADT, installing an already-installed product short-circuits with
// exit code 1638, and any non-install action against a product that is not
// installed short-circuits with 1605 (both with a nil error).
func StartADTMsiProcess(ctx context.Context, opts StartADTMsiProcessOptions) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTMsiProcess: %w", err)
	}
	prep, err := prepareMsiProcess(ctx, &opts)
	if err != nil {
		return nil, err
	}
	if prep.short != nil {
		return prep.short, nil
	}
	res, err := StartADTProcess(ctx, prep.procOpts)
	return finishMsiProcess(opts.SuccessExitCodes == nil, res, err)
}

// StartADTMsiProcessAsUser is the Go port of Start-ADTMsiProcessAsUser: the
// same msiexec composition as StartADTMsiProcess, launched in a logged-on
// user's session via StartADTProcessAsUser (Windows only).
func StartADTMsiProcessAsUser(
	ctx context.Context,
	opts StartADTMsiProcessAsUserOptions,
) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTMsiProcessAsUser: %w", err)
	}
	prep, err := prepareMsiProcess(ctx, &opts.StartADTMsiProcessOptions)
	if err != nil {
		return nil, err
	}
	if prep.short != nil {
		return prep.short, nil
	}
	res, err := StartADTProcessAsUser(ctx, StartADTProcessAsUserOptions{
		StartADTProcessOptions: prep.procOpts,
		UserName:               opts.UserName,
		AllUsers:               opts.AllUsers,
	})
	return finishMsiProcess(opts.SuccessExitCodes == nil, res, err)
}

// StartADTMspProcessOptions mirrors the parameters of Start-ADTMspProcess.
type StartADTMspProcessOptions struct {
	// Path is the .msp patch file; relative paths resolve against the
	// active session's DirFiles.
	Path string
	// AdditionalArgumentList appends to the default MSI parameters.
	AdditionalArgumentList string
	// SecureArgumentList suppresses this facade's argument logging.
	SecureArgumentList bool
	// LoggingOptions overrides config MSI.LoggingOptions.
	LoggingOptions string
	// LogFileName overrides the log file name derived from the patch.
	LogFileName string
	// SuccessExitCodes overrides the exit codes considered successful.
	SuccessExitCodes []int
	// RebootExitCodes overrides the exit codes flagging a reboot.
	RebootExitCodes []int
	// PassThru is accepted for PSADT parity.
	PassThru bool
}

// StartADTMspProcessAsUserOptions mirrors Start-ADTMspProcessAsUser.
type StartADTMspProcessAsUserOptions struct {
	StartADTMspProcessOptions

	// UserName targets a specific logged-on user; empty targets the first
	// active session's user.
	UserName string
	// AllUsers runs the patch in every logged-on user session.
	AllUsers bool
}

// StartADTMspProcess is the Go port of Start-ADTMspProcess: it applies an
// MSP patch by proxying to StartADTMsiProcess with the Patch action, exactly
// like the PowerShell implementation.
func StartADTMspProcess(ctx context.Context, opts StartADTMspProcessOptions) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTMspProcess: %w", err)
	}
	msiOpts, err := opts.msiOptions()
	if err != nil {
		return nil, err
	}
	return StartADTMsiProcess(ctx, msiOpts)
}

// StartADTMspProcessAsUser is the Go port of Start-ADTMspProcessAsUser: the
// Patch action launched in a logged-on user's session (Windows only).
func StartADTMspProcessAsUser(
	ctx context.Context,
	opts StartADTMspProcessAsUserOptions,
) (*ProcessResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: StartADTMspProcessAsUser: %w", err)
	}
	msiOpts, err := opts.msiOptions()
	if err != nil {
		return nil, err
	}
	return StartADTMsiProcessAsUser(ctx, StartADTMsiProcessAsUserOptions{
		StartADTMsiProcessOptions: msiOpts,
		UserName:                  opts.UserName,
		AllUsers:                  opts.AllUsers,
	})
}

// msiOptions validates the MSP options and maps them onto the MSI Patch flow.
func (o StartADTMspProcessOptions) msiOptions() (StartADTMsiProcessOptions, error) {
	if !strings.EqualFold(filepath.Ext(o.Path), ".msp") {
		return StartADTMsiProcessOptions{}, fmt.Errorf(
			"adt: Path %q must be an .msp file: %w", o.Path, ErrInvalidOption)
	}
	return StartADTMsiProcessOptions{
		Action:                 "Patch",
		Path:                   o.Path,
		AdditionalArgumentList: o.AdditionalArgumentList,
		SecureArgumentList:     o.SecureArgumentList,
		LoggingOptions:         o.LoggingOptions,
		LogFileName:            o.LogFileName,
		SuccessExitCodes:       o.SuccessExitCodes,
		RebootExitCodes:        o.RebootExitCodes,
		PassThru:               o.PassThru,
	}, nil
}

// GetADTMsiTablePropertyOptions mirrors the parameters of
// Get-ADTMsiTableProperty for the Property table.
type GetADTMsiTablePropertyOptions struct {
	// Path is the MSI database file.
	Path string
	// Property selects a single Property-table row; empty returns the whole
	// table.
	Property string
}

// GetADTMsiTableProperty is the Go port of Get-ADTMsiTableProperty for the
// Property table: it returns the requested property (a single-entry map) or
// every property of the MSI database (Windows only).
func GetADTMsiTableProperty(
	ctx context.Context,
	opts GetADTMsiTablePropertyOptions,
) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: GetADTMsiTableProperty: %w", err)
	}
	if opts.Path == "" {
		return nil, fmt.Errorf("adt: Path is required: %w", ErrInvalidOption)
	}
	logToSession(fmt.Sprintf("Reading data from Windows Installer database file [%s].", opts.Path),
		LogSeverityInfo, "GetADTMsiTableProperty")
	if opts.Property != "" {
		value, err := msipkg.TableProperty(opts.Path, opts.Property)
		if err != nil {
			return nil, err
		}
		return map[string]string{opts.Property: value}, nil
	}
	props, err := msipkg.AllProperties(opts.Path)
	if err != nil {
		return nil, err
	}
	return props, nil
}

// SetADTMsiPropertyOptions mirrors the parameters of Set-ADTMsiProperty.
type SetADTMsiPropertyOptions struct {
	// Path is the MSI database file to modify.
	Path string
	// PropertyName is the Property-table row to update or insert.
	PropertyName string
	// PropertyValue is the value to write.
	PropertyValue string
}

// SetADTMsiProperty is the Go port of Set-ADTMsiProperty: it opens the MSI
// database writable and updates (or inserts) the Property-table row
// (Windows only).
func SetADTMsiProperty(ctx context.Context, opts SetADTMsiPropertyOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: SetADTMsiProperty: %w", err)
	}
	if opts.Path == "" || opts.PropertyName == "" {
		return fmt.Errorf("adt: Path and PropertyName are required: %w", ErrInvalidOption)
	}
	logToSession(fmt.Sprintf("Setting the MSI Property Name [%s] with Property Value [%s].",
		opts.PropertyName, opts.PropertyValue), LogSeverityInfo, "SetADTMsiProperty")
	return msipkg.SetProperty(opts.Path, opts.PropertyName, opts.PropertyValue)
}

// NewADTMsiTransformOptions mirrors the parameters of New-ADTMsiTransform.
type NewADTMsiTransformOptions struct {
	// MsiPath is the baseline MSI database.
	MsiPath string
	// ApplyTransformPath optionally applies an existing MST before the
	// property changes are captured.
	ApplyTransformPath string
	// NewTransformPath is the MST to create. Defaults to
	// "<ApplyTransformPath>.new.mst" beside the MSI when ApplyTransformPath
	// is set, otherwise "<MsiPath>.mst".
	NewTransformPath string
	// TransformProperties are the Property-table values captured in the
	// transform. At least one is required.
	TransformProperties map[string]string
}

// NewADTMsiTransform is the Go port of New-ADTMsiTransform: it generates an
// MST transform from Property-table differences via
// MsiDatabaseGenerateTransform/MsiCreateTransformSummaryInfo (Windows only).
func NewADTMsiTransform(ctx context.Context, opts NewADTMsiTransformOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: NewADTMsiTransform: %w", err)
	}
	if opts.MsiPath == "" {
		return fmt.Errorf("adt: MsiPath is required: %w", ErrInvalidOption)
	}
	if len(opts.TransformProperties) == 0 {
		return fmt.Errorf("adt: at least one transform property is required: %w", ErrInvalidOption)
	}
	newTransformPath := opts.NewTransformPath
	if newTransformPath == "" {
		newTransformPath = defaultTransformPath(opts.MsiPath, opts.ApplyTransformPath)
	}
	logToSession(fmt.Sprintf("Creating a transform file for MSI [%s].", opts.MsiPath),
		LogSeverityInfo, "NewADTMsiTransform")
	if err := msipkg.CreatePropertyTransform(
		opts.MsiPath, newTransformPath, opts.ApplyTransformPath, opts.TransformProperties,
	); err != nil {
		logToSession(fmt.Sprintf("Failed to create new transform file in path [%s].", newTransformPath),
			LogSeverityError, "NewADTMsiTransform")
		return err
	}
	logToSession(fmt.Sprintf("Successfully created new transform file in path [%s].", newTransformPath),
		LogSeveritySuccess, "NewADTMsiTransform")
	return nil
}

// defaultTransformPath ports New-ADTMsiTransform's default output path.
func defaultTransformPath(msiPath, applyTransformPath string) string {
	dir := filepath.Dir(msiPath)
	if applyTransformPath != "" {
		base := filepath.Base(applyTransformPath)
		ext := filepath.Ext(base)
		return filepath.Join(dir, strings.TrimSuffix(base, ext)+".new"+ext)
	}
	base := filepath.Base(msiPath)
	return filepath.Join(dir, strings.TrimSuffix(base, filepath.Ext(base))+".mst")
}

// GetADTMsiExitCodeMessage is the Go port of Get-ADTMsiExitCodeMessage: it
// returns the descriptive message for an msiexec.exe exit code.
func GetADTMsiExitCodeMessage(code int) string {
	return msipkg.ExitCodeMessage(code)
}

// GetADTApplicationOptions mirrors the parameters of Get-ADTApplication.
type GetADTApplicationOptions struct {
	// Name filters by application display name; empty matches all.
	Name []string
	// NameMatch is Contains (default, ""), Exact, Wildcard or Regex.
	NameMatch string
	// ProductCode filters by MSI product code (any GUID format).
	ProductCode []string
	// ApplicationType is All (default, ""), MSI or EXE.
	ApplicationType string
	// IncludeUpdatesAndHotfixes includes Microsoft update/hotfix entries.
	IncludeUpdatesAndHotfixes bool
}

// GetADTApplication is the Go port of Get-ADTApplication: it enumerates the
// per-user and per-machine uninstall registry keys and returns the installed
// applications matching the filters.
func GetADTApplication(
	ctx context.Context,
	opts GetADTApplicationOptions,
) ([]InstalledApplication, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: GetADTApplication: %w", err)
	}
	logToSession("Getting information for installed applications...",
		LogSeverityInfo, "GetADTApplication")
	apps, err := msipkg.FindInstalledApplications(registryBackend(), msipkg.FindOptions{
		Names:                     opts.Name,
		NameMatch:                 opts.NameMatch,
		ProductCodes:              opts.ProductCode,
		ApplicationType:           opts.ApplicationType,
		IncludeUpdatesAndHotfixes: opts.IncludeUpdatesAndHotfixes,
	})
	if err != nil {
		return nil, err
	}
	for _, app := range apps {
		logToSession(fmt.Sprintf("Found installed application [%s].", applicationLabel(app)),
			LogSeverityInfo, "GetADTApplication")
	}
	if len(apps) == 0 {
		logToSession("Found no application based on the supplied input.",
			LogSeverityInfo, "GetADTApplication")
	}
	return apps, nil
}

// UninstallADTApplicationOptions mirrors the parameters of
// Uninstall-ADTApplication.
type UninstallADTApplicationOptions struct {
	// Name filters by application display name.
	Name []string
	// NameMatch is Contains (default, ""), Exact, Wildcard or Regex.
	NameMatch string
	// ProductCode filters by MSI product code (any GUID format).
	ProductCode []string
	// ArgumentList overrides the default uninstall arguments (config MSI
	// parameters for MSI apps, the registered uninstall string's own
	// arguments for EXE apps).
	ArgumentList string
	// AdditionalArgumentList appends to the uninstall arguments.
	AdditionalArgumentList string
	// ApplicationType is All (default, ""), MSI or EXE.
	ApplicationType string
	// FilterScript further filters the matched applications; nil keeps all.
	FilterScript func(InstalledApplication) bool
}

// UninstallADTApplication is the Go port of Uninstall-ADTApplication: it
// finds the applications matching the filters and removes each one — MSI
// products via StartADTMsiProcess (msiexec /x by product code) and EXE
// products via their QuietUninstallString/UninstallString. Failures are
// logged and collected; remaining applications are still removed.
func UninstallADTApplication(ctx context.Context, opts UninstallADTApplicationOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: UninstallADTApplication: %w", err)
	}
	if len(opts.Name) == 0 && len(opts.ProductCode) == 0 && opts.FilterScript == nil {
		return fmt.Errorf("adt: Name, ProductCode or FilterScript is required: %w", ErrInvalidOption)
	}
	apps, err := GetADTApplication(ctx, GetADTApplicationOptions{
		Name:            opts.Name,
		NameMatch:       opts.NameMatch,
		ProductCode:     opts.ProductCode,
		ApplicationType: opts.ApplicationType,
	})
	if err != nil {
		return err
	}
	if opts.FilterScript != nil {
		filtered := make([]InstalledApplication, 0, len(apps))
		for _, app := range apps {
			if opts.FilterScript(app) {
				filtered = append(filtered, app)
			}
		}
		apps = filtered
	}
	if len(apps) == 0 {
		logToSession("No applications found for removal.", LogSeverityInfo, "UninstallADTApplication")
		return nil
	}
	var errs []error
	for _, app := range apps {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("adt: UninstallADTApplication: %w", err))
			break
		}
		var err error
		if app.WindowsInstaller {
			err = uninstallMsiApplication(ctx, app, &opts)
		} else {
			err = uninstallExeApplication(ctx, app, &opts)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("adt: uninstalling [%s]: %w", app.DisplayName, err))
		}
	}
	return errors.Join(errs...)
}

// uninstallMsiApplication removes one MSI product by product code.
func uninstallMsiApplication(
	ctx context.Context,
	app InstalledApplication,
	opts *UninstallADTApplicationOptions,
) error {
	if app.ProductCode == "" {
		logToSession(fmt.Sprintf("No ProductCode found for MSI application [%s]. Skipping removal.",
			applicationLabel(app)), LogSeverityWarning, "UninstallADTApplication")
		return nil
	}
	logToSession(fmt.Sprintf("Removing MSI application [%s] with ProductCode [%s].",
		applicationLabel(app), app.ProductCode), LogSeverityInfo, "UninstallADTApplication")
	_, err := StartADTMsiProcess(ctx, StartADTMsiProcessOptions{
		Action:                       "Uninstall",
		Path:                         app.ProductCode,
		ArgumentList:                 opts.ArgumentList,
		AdditionalArgumentList:       opts.AdditionalArgumentList,
		SkipMSIAlreadyInstalledCheck: true,
	})
	return err
}

// uninstallExeApplication removes one EXE-based product via its registered
// quiet (preferred) or standard uninstall string.
func uninstallExeApplication(
	ctx context.Context,
	app InstalledApplication,
	opts *UninstallADTApplicationOptions,
) error {
	command := app.QuietUninstallString
	if strings.TrimSpace(strings.ReplaceAll(command, `"`, "")) == "" {
		command = app.UninstallString
	}
	if strings.TrimSpace(strings.ReplaceAll(command, `"`, "")) == "" {
		logToSession(fmt.Sprintf("No UninstallString found for EXE application [%s]. Skipping removal.",
			applicationLabel(app)), LogSeverityWarning, "UninstallADTApplication")
		return nil
	}
	filePath, args := splitUninstallCommand(command)
	if opts.ArgumentList != "" {
		args = opts.ArgumentList
	}
	if opts.AdditionalArgumentList != "" {
		args = strings.TrimSpace(args + " " + opts.AdditionalArgumentList)
	}
	logToSession(fmt.Sprintf("Removing EXE application [%s].", applicationLabel(app)),
		LogSeverityInfo, "UninstallADTApplication")
	_, err := StartADTProcess(ctx, StartADTProcessOptions{
		FilePath:       filePath,
		ArgumentList:   args,
		WaitForMsiExec: true,
	})
	return err
}

// applicationLabel formats "DisplayName DisplayVersion" the way PSADT logs
// applications (the version is omitted when the name already contains it).
func applicationLabel(app InstalledApplication) string {
	if app.DisplayVersion != "" && !strings.Contains(app.DisplayName, app.DisplayVersion) {
		return app.DisplayName + " " + app.DisplayVersion
	}
	return app.DisplayName
}

// splitUninstallCommand splits a registered uninstall command line into the
// executable path and its arguments, honoring a quoted executable path.
func splitUninstallCommand(command string) (filePath, args string) {
	command = strings.TrimSpace(command)
	if strings.HasPrefix(command, `"`) {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			return command[1 : end+1], strings.TrimSpace(command[end+2:])
		}
		return strings.Trim(command, `"`), ""
	}
	if i := strings.IndexByte(command, ' '); i >= 0 {
		return command[:i], strings.TrimSpace(command[i+1:])
	}
	return command, ""
}

// msiPreparation is the resolved launch strategy of one Start-ADTMsiProcess
// invocation: either a short-circuit result (already installed / not
// installed) or the StartADTProcess options to execute.
type msiPreparation struct {
	procOpts StartADTProcessOptions
	short    *ProcessResult
}

// prepareMsiProcess ports Start-ADTMsiProcess's begin/process blocks: action
// validation, path/product resolution, the installed-product check, log file
// derivation and command-line composition.
func prepareMsiProcess(ctx context.Context, opts *StartADTMsiProcessOptions) (*msiPreparation, error) {
	action, err := canonicalMsiAction(opts.Action)
	if err != nil {
		return nil, err
	}
	if opts.Path == "" {
		return nil, fmt.Errorf("adt: Path is required: %w", ErrInvalidOption)
	}
	if action == "Install" && msipkg.IsProductCode(opts.Path) {
		return nil, fmt.Errorf(
			"adt: a product code can only be used with non-install actions: %w", ErrInvalidOption)
	}
	s, _ := GetADTSession() // nil session means sessionless operation
	logToSession(fmt.Sprintf("Executing MSI action [%s]...", action),
		LogSeverityInfo, "StartADTMsiProcess")
	product, err := resolveMsiProduct(s, opts.Path)
	if err != nil {
		return nil, err
	}
	transforms := resolveMsiTransforms(product, opts.Transforms)

	installed := msiInstalledState(ctx, action, product, opts)
	switch {
	case installed && action == "Install":
		logToSession("The MSI is already installed on this system, skipping action [Install]...",
			LogSeverityInfo, "StartADTMsiProcess")
		return &msiPreparation{short: &ProcessResult{ExitCode: 1638}}, nil
	case !installed && action != "Install":
		logToSession(fmt.Sprintf("The MSI is not installed on this system, skipping action [%s]...", action),
			LogSeverityInfo, "StartADTMsiProcess")
		return &msiPreparation{short: &ProcessResult{ExitCode: 1605}}, nil
	}

	cfg := activeMsiConfig(s)
	silent := s != nil && (s.IsSilent() || s.IsNonInteractive())
	logFile := resolveMsiLogFile(s, cfg, opts.LogFileName, product, action)
	if logFile != "" {
		_ = os.MkdirAll(filepath.Dir(logFile), 0o755) // best effort, msiexec reports 1622 itself
	}
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:                 action,
		Product:                product,
		Transforms:             transforms,
		ArgumentList:           opts.ArgumentList,
		AdditionalArgumentList: opts.AdditionalArgumentList,
		LoggingOptions:         opts.LoggingOptions,
		LogFile:                logFile,
		Silent:                 silent,
	}, cfg.MSI)
	if err != nil {
		return nil, err
	}
	if !opts.SecureArgumentList {
		logToSession(fmt.Sprintf("Composed msiexec arguments [%s].", args),
			LogSeverityInfo, "StartADTMsiProcess")
	}
	return &msiPreparation{procOpts: StartADTProcessOptions{
		FilePath:         msiexecPath(),
		ArgumentList:     args,
		WaitForMsiExec:   true,
		SuccessExitCodes: opts.SuccessExitCodes,
		RebootExitCodes:  opts.RebootExitCodes,
		PassThru:         opts.PassThru,
	}}, nil
}

// finishMsiProcess logs the MSI exit-code message and, when the default exit
// code lists are in play, forgives exit codes the MSI success set (msipkg)
// covers beyond the generic process defaults (e.g. 1707).
func finishMsiProcess(defaultCodes bool, res *ProcessResult, err error) (*ProcessResult, error) {
	if res == nil {
		return nil, err
	}
	severity := LogSeverityInfo
	if !msipkg.IsSuccessExitCode(res.ExitCode) {
		severity = LogSeverityError
	}
	logToSession(fmt.Sprintf("MSI operation completed with exit code [%d]: %s",
		res.ExitCode, msipkg.ExitCodeMessage(res.ExitCode)), severity, "StartADTMsiProcess")
	if err != nil && defaultCodes && msipkg.IsSuccessExitCode(res.ExitCode) {
		return res, nil
	}
	return res, err
}

// msiInstalledState ports Start-ADTMsiProcess's already-installed check: when
// a product code is known (given directly or read from the MSI's Property
// table), the uninstall registry decides; otherwise the action's requirement
// is assumed satisfied, so the operation proceeds.
func msiInstalledState(
	ctx context.Context,
	action, product string,
	opts *StartADTMsiProcessOptions,
) bool {
	assumed := action != "Install"
	if opts.SkipMSIAlreadyInstalledCheck {
		return assumed
	}
	code := ""
	if msipkg.IsProductCode(product) {
		code = product
	} else if strings.EqualFold(filepath.Ext(product), ".msi") {
		logToSession("Determining whether the MSI is already installed on this system.",
			LogSeverityInfo, "StartADTMsiProcess")
		if pc, err := msipkg.TableProperty(product, "ProductCode"); err == nil {
			if normalized, err := msipkg.NormalizeProductCode(pc); err == nil {
				code = normalized
			}
		}
	}
	if code == "" {
		return assumed
	}
	apps, err := GetADTApplication(ctx, GetADTApplicationOptions{ProductCode: []string{code}})
	if err != nil {
		return assumed
	}
	return len(apps) > 0
}

// canonicalMsiAction validates and canonicalizes the MSI action verb.
func canonicalMsiAction(action string) (string, error) {
	if action == "" {
		return "Install", nil
	}
	for _, valid := range []string{"Install", "Uninstall", "Repair", "Patch", "ActiveSetup"} {
		if strings.EqualFold(action, valid) {
			return valid, nil
		}
	}
	return "", fmt.Errorf("adt: Action %q: %w", action, ErrInvalidOption)
}

// resolveMsiProduct normalizes a product code, or resolves the MSI/MSP file
// path (relative paths resolve against the session's DirFiles like
// Start-ADTMsiProcess).
func resolveMsiProduct(s *DeploymentSession, path string) (string, error) {
	if msipkg.IsProductCode(path) {
		return msipkg.NormalizeProductCode(path)
	}
	if _, err := os.Stat(path); err == nil {
		if abs, err := filepath.Abs(path); err == nil {
			return abs, nil
		}
		return path, nil
	}
	if !filepath.IsAbs(path) && s != nil && s.DirFiles() != "" {
		candidate := filepath.Join(s.DirFiles(), path)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("adt: failed to find the file [%s]: %w", path, ErrNotFound)
}

// resolveMsiTransforms resolves relative transform names against the MSI's
// directory when the transform file exists there, like Start-ADTMsiProcess.
func resolveMsiTransforms(product string, transforms []string) []string {
	if len(transforms) == 0 || msipkg.IsProductCode(product) {
		return transforms
	}
	dir := filepath.Dir(product)
	resolved := make([]string, 0, len(transforms))
	for _, transform := range transforms {
		name := strings.TrimPrefix(transform, `.\`)
		if !filepath.IsAbs(name) {
			if candidate := filepath.Join(dir, name); fileExists(candidate) {
				resolved = append(resolved, candidate)
				continue
			}
		}
		resolved = append(resolved, name)
	}
	return resolved
}

// fileExists reports whether path exists as a file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// activeMsiConfig returns the session's config, or the embedded defaults
// (with paths expanded) when operating sessionless.
func activeMsiConfig(s *DeploymentSession) *config.Config {
	if s != nil {
		return s.Config()
	}
	cfg, err := config.Default()
	if err != nil {
		return &config.Config{}
	}
	cfg.ExpandPaths()
	return cfg
}

// msiArgumentInputs are the resolved inputs of buildMsiArguments.
type msiArgumentInputs struct {
	// Action is the canonical MSI action verb.
	Action string
	// Product is the resolved MSI/MSP path or "{GUID}" product code.
	Product string
	// Transforms are the resolved MST paths for TRANSFORMS=.
	Transforms []string
	// ArgumentList replaces the config default parameters when non-empty.
	ArgumentList string
	// AdditionalArgumentList appends to the parameters.
	AdditionalArgumentList string
	// LoggingOptions overrides the config MSI logging switches.
	LoggingOptions string
	// LogFile is the full MSI log path; empty disables MSI logging switches.
	LogFile string
	// Silent selects config SilentParams over Install/UninstallParams.
	Silent bool
}

// buildMsiArguments composes the msiexec.exe argument string the way
// Start-ADTMsiProcess does: the action switch and target, TRANSFORMS
// properties, the config default parameters (which carry
// REBOOT=ReallySuppress) unless overridden, any additional arguments, and
// the verbose logging switches.
func buildMsiArguments(in msiArgumentInputs, msiCfg config.MSI) (string, error) {
	installParams, uninstallParams := msiCfg.InstallParams, msiCfg.UninstallParams
	if in.Silent {
		installParams, uninstallParams = msiCfg.SilentParams, msiCfg.SilentParams
	}
	var option, defaults string
	switch in.Action {
	case "Install":
		option, defaults = "/i", installParams
	case "Uninstall":
		option, defaults = "/x", uninstallParams
	case "Patch":
		option, defaults = "/p", installParams
	case "Repair":
		option, defaults = "/fomus", installParams
	case "ActiveSetup":
		option, defaults = "/fups", ""
	default:
		return "", fmt.Errorf("adt: Action %q: %w", in.Action, ErrInvalidOption)
	}
	parts := make([]string, 0, 8)
	parts = append(parts, option, quoteMsiProduct(in.Product))
	if len(in.Transforms) > 0 {
		parts = append(parts,
			`TRANSFORMS="`+strings.Join(in.Transforms, ";")+`"`,
			"TRANSFORMSSECURE=1")
	}
	switch {
	case in.ArgumentList != "":
		parts = append(parts, in.ArgumentList)
	case defaults != "":
		parts = append(parts, defaults)
	}
	if in.AdditionalArgumentList != "" {
		parts = append(parts, in.AdditionalArgumentList)
	}
	if in.LogFile != "" {
		loggingOptions := in.LoggingOptions
		if loggingOptions == "" {
			loggingOptions = msiCfg.LoggingOptions
		}
		if loggingOptions == "" {
			loggingOptions = "/L*V"
		}
		parts = append(parts, loggingOptions, `"`+in.LogFile+`"`)
	}
	return strings.Join(parts, " "), nil
}

// quoteMsiProduct quotes file paths for the msiexec command line; product
// codes are passed bare.
func quoteMsiProduct(product string) string {
	if strings.HasPrefix(product, "{") {
		return product
	}
	return `"` + product + `"`
}

// msiLogNameSanitizer strips characters that are invalid in file names and,
// matching PSADT's derived MSI log names, all whitespace.
var msiLogNameSanitizer = regexp.MustCompile(`[\\/:*?"<>|]|\s+`)

// msiLogExtensions are the log extensions Start-ADTMsiProcess recognizes on
// a caller-provided LogFileName.
var msiLogExtensions = []string{".log", ".logx", ".txt", ".out"}

// resolveMsiLogFile ports Start-ADTMsiProcess's log path derivation: the
// caller-provided LogFileName (or a name derived from the MSI/product code)
// gets the action appended and lands in config MSI.LogPath, the session's
// log directory, or config Toolkit.LogPath, in that order. An empty result
// disables MSI logging (session logging disabled and no LogFileName given,
// or no resolvable log directory).
func resolveMsiLogFile(s *DeploymentSession, cfg *config.Config, logFileName, product, action string) string {
	name := strings.TrimSpace(logFileName)
	ext := ""
	for _, known := range msiLogExtensions {
		if strings.HasSuffix(strings.ToLower(name), known) {
			ext = name[len(name)-len(known):]
			name = name[:len(name)-len(known)]
			break
		}
	}
	if name == "" {
		if s != nil && s.Options().DisableLogging {
			return ""
		}
		base := product
		if !msipkg.IsProductCode(product) {
			base = pathBaseName(product)
			base = strings.TrimSuffix(base, filepath.Ext(base))
		}
		name = msiLogNameSanitizer.ReplaceAllString(base, "")
	}
	if name == "" {
		return ""
	}
	if ext == "" {
		ext = ".log"
	}
	if !strings.HasSuffix(strings.ToLower(name), strings.ToLower(action)) {
		name += "_" + action
	}
	if filepath.IsAbs(name) {
		return name + ext
	}
	dir := config.ExpandEnv(cfg.MSI.LogPath)
	if dir == "" {
		if s != nil && s.LogPath() != "" {
			dir = filepath.Dir(s.LogPath())
		} else {
			dir = config.ExpandEnv(cfg.Toolkit.LogPath)
		}
	}
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, name+ext)
}

// pathBaseName returns the last element of a path, accepting both Windows
// and POSIX separators so Windows-style package paths behave identically in
// portable tests.
func pathBaseName(path string) string {
	if i := strings.LastIndexAny(path, `\/`); i >= 0 {
		return path[i+1:]
	}
	return path
}

// msiexecPath returns the Windows Installer executable path
// (%SystemRoot%\System32\msiexec.exe, matching PSADT's use of the system
// directory), falling back to a bare name for PATH resolution.
func msiexecPath() string {
	if root := os.Getenv("SystemRoot"); root != "" {
		return filepath.Join(root, "System32", "msiexec.exe")
	}
	return "msiexec.exe"
}
