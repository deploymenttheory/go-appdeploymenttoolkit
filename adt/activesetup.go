package adt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// SetADTActiveSetupOptions mirrors the parameters of Set-ADTActiveSetup.
type SetADTActiveSetupOptions struct {
	// StubExePath is the destination path of the file executed on user login
	// (.exe, .vbs, .cmd, .bat, .ps1 or .js). Required unless
	// PurgeActiveSetupKey is set.
	StubExePath string
	// ArgumentList are the arguments passed to the executed file.
	ArgumentList []string
	// Key is the Active Setup component ID (the registry key name). Defaults
	// to the active session's InstallName; required when sessionless.
	Key string
	// Description is the (Default) value shown at logon. Defaults to the
	// active session's InstallName; required when sessionless.
	Description string
	// Version pins the Active Setup Version value. Defaults to a timestamp.
	Version string
	// Locale is an arbitrary language tag stored under the HKLM entry.
	Locale string
	// ExecutionPolicy sets the PowerShell execution policy when StubExePath
	// is a .ps1 script.
	ExecutionPolicy string
	// Wow6432Node writes the entry under the 32-bit registry view.
	Wow6432Node bool
	// DisableActiveSetup writes IsInstalled=0 so the StubPath is not executed;
	// this also implies NoExecuteForCurrentUser.
	DisableActiveSetup bool
	// NoExecuteForCurrentUser suppresses running the StubPath for the current
	// user after writing the registry entry.
	NoExecuteForCurrentUser bool
	// PurgeActiveSetupKey removes the Active Setup entry (HKLM and all user
	// hives) and returns.
	PurgeActiveSetupKey bool
}

// SetADTActiveSetup is the Go port of Set-ADTActiveSetup: it creates (or
// purges) an Active Setup entry under
// HKLM\SOFTWARE\Microsoft\Active Setup\Installed Components\<Key> so a stub is
// executed for each user at logon, and optionally executes it immediately for
// the current user.
//
// Deviations from PSADT: this port always executes the current-user stub via
// StartADTProcess in the current session (PSADT additionally handles the
// Session-0/SYSTEM case with Start-ADTProcessAsUser and an HKLM-vs-HKCU
// version comparison), and it does not return a ProcessResult (PassThru).
func SetADTActiveSetup(ctx context.Context, opts SetADTActiveSetupOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: SetADTActiveSetup: %w", err)
	}
	key, description, err := resolveActiveSetupIdentity(opts)
	if err != nil {
		return err
	}
	osIs64, _ := systemBitness()
	wow := opts.Wow6432Node && osIs64
	hklmKey := activeSetupKeyPath("HKLM", key, wow)
	hkcuKey := activeSetupKeyPath("HKCU", key, wow)

	if opts.PurgeActiveSetupKey {
		return purgeActiveSetup(ctx, hklmKey, hkcuKey)
	}

	if strings.TrimSpace(opts.StubExePath) == "" {
		return fmt.Errorf("adt: StubExePath is required: %w", ErrInvalidOption)
	}
	version := opts.Version
	if version == "" {
		version = formatActiveSetupVersion(time.Now())
	}

	copyActiveSetupStub(ctx, opts.StubExePath)
	if err := verifyActiveSetupStub(opts.StubExePath); err != nil {
		return err
	}

	stub, err := buildActiveSetupStub(opts.StubExePath, opts.ArgumentList, opts.ExecutionPolicy)
	if err != nil {
		return err
	}

	logToSession(fmt.Sprintf("Adding Active Setup Key for local machine: [%s].", hklmKey),
		LogSeverityInfo, "SetADTActiveSetup")
	if err := writeActiveSetupEntry(ctx, hklmKey, description, version, stub.StubPath, opts.Locale, true, opts.DisableActiveSetup); err != nil {
		return err
	}

	if opts.NoExecuteForCurrentUser || opts.DisableActiveSetup {
		return nil
	}

	logToSession("Executing Active Setup StubPath file for the current user.",
		LogSeverityInfo, "SetADTActiveSetup")
	if _, err := StartADTProcess(ctx, StartADTProcessOptions{
		FilePath:       stub.CUStubExePath,
		ArgumentList:   stub.CUArguments,
		CreateNoWindow: true,
	}); err != nil {
		return err
	}

	logToSession(fmt.Sprintf("Adding Active Setup Key for the current user: [%s].", hkcuKey),
		LogSeverityInfo, "SetADTActiveSetup")
	return writeActiveSetupEntry(ctx, hkcuKey, description, version, stub.StubPath, opts.Locale, false, opts.DisableActiveSetup)
}

// resolveActiveSetupIdentity resolves the component Key and Description,
// defaulting to the active session's InstallName and erroring when neither is
// available.
func resolveActiveSetupIdentity(opts SetADTActiveSetupOptions) (key, description string, err error) {
	key, description = opts.Key, opts.Description
	if key == "" || (description == "" && !opts.PurgeActiveSetupKey) {
		if s, serr := GetADTSession(); serr == nil {
			if key == "" {
				key = s.InstallName()
			}
			if description == "" {
				description = s.InstallName()
			}
		}
	}
	if strings.TrimSpace(key) == "" {
		return "", "", fmt.Errorf("adt: Key is required when no active session: %w", ErrInvalidOption)
	}
	if !opts.PurgeActiveSetupKey && strings.TrimSpace(description) == "" {
		return "", "", fmt.Errorf("adt: Description is required when no active session: %w", ErrInvalidOption)
	}
	return key, description, nil
}

// purgeActiveSetup removes the Active Setup entry from HKLM and from every
// loaded user hive under HKEY_USERS.
func purgeActiveSetup(ctx context.Context, hklmKey, hkcuKey string) error {
	logToSession(fmt.Sprintf("Removing Active Setup entry [%s].", hklmKey),
		LogSeverityInfo, "SetADTActiveSetup")
	if err := RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: hklmKey, Recurse: true}); err != nil {
		return err
	}
	logToSession(fmt.Sprintf("Removing Active Setup entry [%s] for all logged on user registry hives on the system.", hkcuKey),
		LogSeverityInfo, "SetADTActiveSetup")
	perUserKey := activeSetupPerUserSubkey(hkcuKey)
	return InvokeADTAllUsersRegistryAction(ctx, func(ctx context.Context, userSID string) error {
		return RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{
			Key:     `HKU\` + userSID + `\` + perUserKey,
			Recurse: true,
		})
	})
}

// activeSetupPerUserSubkey strips the HKCU root from an Active Setup key path,
// yielding the "Software\...\<Key>" subkey applied under each HKU\<SID> hive.
func activeSetupPerUserSubkey(hkcuKey string) string {
	rest := strings.TrimPrefix(hkcuKey, "HKCU")
	rest = strings.TrimPrefix(rest, ":")
	return strings.TrimPrefix(rest, `\`)
}

// activeSetupKeyPath composes the Active Setup registry key path for the given
// hive ("HKLM" or "HKCU"), honoring the 32-bit registry view.
func activeSetupKeyPath(hive, key string, wow bool) string {
	software := "SOFTWARE"
	if hive == "HKCU" {
		software = "Software"
	}
	segments := []string{hive, software}
	if wow {
		segments = append(segments, "Wow6432Node")
	}
	segments = append(segments, "Microsoft", "Active Setup", "Installed Components", key)
	return strings.Join(segments, `\`)
}

// writeActiveSetupEntry writes the standard Active Setup values. IsInstalled is
// written only for the HKLM entry, matching Set-ADTActiveSetupRegistryEntry.
func writeActiveSetupEntry(
	ctx context.Context,
	regPath, description, version, stubPath, locale string,
	isHKLM, disable bool,
) error {
	for _, entry := range activeSetupRegistryValues(description, version, stubPath, locale, isHKLM, disable) {
		if err := SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
			Key:   regPath,
			Name:  entry.name,
			Value: entry.value,
			Type:  entry.kind,
		}); err != nil {
			return err
		}
	}
	return nil
}

// activeSetupRegistryEntry is one registry value written for an Active Setup
// component.
type activeSetupRegistryEntry struct {
	name  string
	value any
	kind  RegistryValueKind
}

// activeSetupRegistryValues composes the ordered set of registry values for an
// Active Setup entry, mirroring Set-ADTActiveSetupRegistryEntry: (Default) gets
// the description, Version has its dots turned into commas, StubPath is an
// expandable string, Locale is optional and IsInstalled (HKLM only) reflects
// the DisableActiveSetup switch.
func activeSetupRegistryValues(
	description, version, stubPath, locale string,
	isHKLM, disable bool,
) []activeSetupRegistryEntry {
	entries := []activeSetupRegistryEntry{
		{name: "(Default)", value: description, kind: RegistryValueKindString},
		{name: "Version", value: strings.ReplaceAll(version, ".", ","), kind: RegistryValueKindString},
		{name: "StubPath", value: stubPath, kind: RegistryValueKindExpandString},
	}
	if strings.TrimSpace(locale) != "" {
		entries = append(entries, activeSetupRegistryEntry{name: "Locale", value: locale, kind: RegistryValueKindString})
	}
	if isHKLM {
		installed := uint32(1)
		if disable {
			installed = 0
		}
		entries = append(entries, activeSetupRegistryEntry{name: "IsInstalled", value: installed, kind: RegistryValueKindDWord})
	}
	return entries
}

// activeSetupStub carries the resolved current-user launcher and the StubPath
// registry string.
type activeSetupStub struct {
	CUStubExePath string
	CUArguments   string
	StubPath      string
}

// activeSetupPercentVar matches an environment variable reference (%NAME%).
var activeSetupPercentVar = regexp.MustCompile(`%\w+%`)

// buildActiveSetupStub ports Set-ADTActiveSetup's per-extension StubPath
// composition, returning the launcher, its arguments and the StubPath string
// recorded in the registry.
func buildActiveSetupStub(stubExePath string, argumentList []string, executionPolicy string) (activeSetupStub, error) {
	arguments := joinActiveSetupArguments(argumentList)
	sysDir := activeSetupSystemDir()
	ext := strings.ToLower(filepath.Ext(stubExePath))
	switch ext {
	case ".exe":
		stub := activeSetupStub{CUStubExePath: stubExePath, CUArguments: arguments}
		if arguments == "" {
			stub.StubPath = `"` + stubExePath + `"`
		} else {
			stub.StubPath = `"` + stubExePath + `" ` + arguments
		}
		return stub, nil
	case ".js", ".vbs":
		cu := filepath.Join(sysDir, "wscript.exe")
		var cuArgs string
		if arguments == "" {
			cuArgs = `//nologo "` + stubExePath + `"`
		} else {
			cuArgs = `//nologo "` + stubExePath + `" ` + arguments
		}
		return activeSetupStub{CUStubExePath: cu, CUArguments: cuArgs, StubPath: `"` + cu + `" ` + cuArgs}, nil
	case ".cmd", ".bat":
		cu := filepath.Join(sysDir, "cmd.exe")
		escaped := escapeCmdMetacharacters(stubExePath)
		var cuArgs string
		if arguments == "" {
			cuArgs = `/C "` + escaped + `"`
		} else {
			cuArgs = `/C ""` + escaped + `" ` + arguments + `"`
		}
		return activeSetupStub{CUStubExePath: cu, CUArguments: cuArgs, StubPath: `"` + cu + `" ` + cuArgs}, nil
	case ".ps1":
		cu := activeSetupPowerShellPath()
		prefix := ""
		if strings.TrimSpace(executionPolicy) != "" {
			prefix = "-ExecutionPolicy " + executionPolicy + " "
		}
		cuArgs := prefix + `-NoProfile -NoLogo -WindowStyle Hidden -File "` + stubExePath + `"`
		if arguments != "" {
			cuArgs += " " + arguments
		}
		return activeSetupStub{CUStubExePath: cu, CUArguments: cuArgs, StubPath: `"` + cu + `" ` + cuArgs}, nil
	default:
		return activeSetupStub{}, fmt.Errorf(
			"adt: StubExePath extension %q is not supported (.exe/.vbs/.cmd/.bat/.ps1/.js): %w", ext, ErrInvalidOption)
	}
}

// joinActiveSetupArguments composes the argument string from the list: a single
// argument is used verbatim, multiple arguments are individually quoted and
// space-joined, mirroring CommandLineUtilities.ArgumentListToCommandLine.
func joinActiveSetupArguments(args []string) string {
	switch len(args) {
	case 0:
		return ""
	case 1:
		return args[0]
	default:
		quoted := make([]string, 0, len(args))
		for _, a := range args {
			quoted = append(quoted, quoteActiveSetupArgument(a))
		}
		return strings.Join(quoted, " ")
	}
}

// quoteActiveSetupArgument quotes an argument that contains whitespace or a
// quote, escaping embedded quotes by doubling.
func quoteActiveSetupArgument(arg string) string {
	if arg == "" {
		return `""`
	}
	if !strings.ContainsAny(arg, " \t\"") {
		return arg
	}
	return `"` + strings.ReplaceAll(arg, `"`, `""`) + `"`
}

// cmdMetacharsWithSpace / cmdMetacharsNoSpace port Set-ADTActiveSetup's cmd.exe
// metacharacter escaping: parentheses only require escaping when the path has
// no whitespace.
var (
	cmdMetacharsWithSpace = regexp.MustCompile(`([&^])`)
	cmdMetacharsNoSpace   = regexp.MustCompile(`([()&^])`)
)

// escapeCmdMetacharacters prefixes cmd.exe metacharacters with a caret, exactly
// as Set-ADTActiveSetup does for .cmd/.bat stubs.
func escapeCmdMetacharacters(stubExePath string) string {
	if strings.ContainsAny(strings.TrimSpace(stubExePath), " \t") {
		return cmdMetacharsWithSpace.ReplaceAllString(stubExePath, `^$1`)
	}
	return cmdMetacharsNoSpace.ReplaceAllString(stubExePath, `^$1`)
}

// formatActiveSetupVersion renders the default Active Setup Version string in
// PSADT's "yyMM,ddHH,mmss" format, which avoids the >8-consecutive-digit
// Windows limitation.
func formatActiveSetupVersion(t time.Time) string {
	return t.Format("0601,0215,0405")
}

// copyActiveSetupStub copies the stub from the active session's Files directory
// to the destination when present, mirroring Set-ADTActiveSetup's Copy-ADTFile.
func copyActiveSetupStub(ctx context.Context, stubExePath string) {
	s, err := GetADTSession()
	if err != nil || s.DirFiles() == "" {
		return
	}
	src := filepath.Join(s.DirFiles(), filepath.Base(stubExePath))
	if info, err := os.Stat(src); err == nil && !info.IsDir() {
		_ = CopyADTFile(ctx, CopyADTFileOptions{Path: []string{src}, Destination: stubExePath})
	}
}

// verifyActiveSetupStub ensures the stub file exists unless the path contains
// an unexpanded environment variable reference.
func verifyActiveSetupStub(stubExePath string) error {
	if activeSetupPercentVar.MatchString(stubExePath) {
		return nil
	}
	if info, err := os.Stat(stubExePath); err != nil || info.IsDir() {
		return fmt.Errorf("adt: Active Setup StubPath file [%s] is missing: %w",
			filepath.Base(stubExePath), ErrNotFound)
	}
	return nil
}

// activeSetupSystemDir returns %WINDIR%\System32.
func activeSetupSystemDir() string {
	if windir := windowsDir(); windir != "" {
		return filepath.Join(windir, "System32")
	}
	return `C:\Windows\System32`
}

// activeSetupPowerShellPath returns the Windows PowerShell executable path,
// matching Get-ADTPowerShellProcessPath for the built-in engine.
func activeSetupPowerShellPath() string {
	return filepath.Join(activeSetupSystemDir(), "WindowsPowerShell", "v1.0", "powershell.exe")
}
