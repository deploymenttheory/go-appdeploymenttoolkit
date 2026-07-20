package winadt

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// RegistryValueKind mirrors Microsoft.Win32.RegistryValueKind for the value
// types PSADT writes. The zero value means "infer the kind from the Go type
// of the supplied value".
type RegistryValueKind int

// RegistryValueKind values.
const (
	RegistryValueKindInferred RegistryValueKind = iota
	RegistryValueKindString
	RegistryValueKindExpandString
	RegistryValueKindDWord
	RegistryValueKindQWord
	RegistryValueKindMultiString
	RegistryValueKindBinary
)

// registryBackendHook lets tests inject a regkey.Fake in place of the
// platform default backend. Nil selects the default (Native on Windows,
// Fake elsewhere).
var registryBackendHook func() regkey.Backend

// registryBackend resolves the registry backend for the facade functions:
// the test hook when set, otherwise the platform default.
func registryBackend() regkey.Backend {
	if hook := registryBackendHook; hook != nil {
		return hook()
	}
	return defaultRegistryBackend()
}

// Registry path transformation tables ported from PSADT's $Script:Registry
// constants (ImportsLast.ps1).
var (
	// hiveAbbreviations expands PSADT/PowerShell drive-style hive prefixes
	// ("HKLM:\", "HKLM:", "HKLM\") to their full root key names. Order
	// matters: longer abbreviations are matched before their prefixes.
	hiveAbbreviations = []struct{ abbrev, full string }{
		{"HKLM", "HKEY_LOCAL_MACHINE"},
		{"HKCR", "HKEY_CLASSES_ROOT"},
		{"HKCU", "HKEY_CURRENT_USER"},
		{"HKCC", "HKEY_CURRENT_CONFIG"},
		{"HKPD", "HKEY_PERFORMANCE_DATA"},
		{"HKU", "HKEY_USERS"},
	}

	// wow64Replacements port PSADT's WOW64Replacements table: the first
	// matching rule rewrites the path for the 32-bit registry view.
	wow64Replacements = []struct {
		pattern *regexp.Regexp
		replace string
	}{
		{
			regexp.MustCompile(`(?i)^(HKEY_LOCAL_MACHINE\\SOFTWARE\\Classes\\|HKEY_CURRENT_USER\\SOFTWARE\\Classes\\|HKEY_CLASSES_ROOT\\)(AppID\\|CLSID\\|DirectShow\\|Interface\\|Media Type\\|MediaFoundation\\|PROTOCOLS\\|TypeLib\\)`),
			`${1}Wow6432Node\${2}`,
		},
		{
			regexp.MustCompile(`(?i)^HKEY_LOCAL_MACHINE\\SOFTWARE\\`),
			`HKEY_LOCAL_MACHINE\SOFTWARE\Wow6432Node\`,
		},
		{
			regexp.MustCompile(`(?i)^HKEY_LOCAL_MACHINE\\SOFTWARE$`),
			`HKEY_LOCAL_MACHINE\SOFTWARE\Wow6432Node`,
		},
		{
			regexp.MustCompile(`(?i)^HKEY_CURRENT_USER\\Software\\Microsoft\\Active Setup\\Installed Components\\`),
			`HKEY_CURRENT_USER\Software\Wow6432Node\Microsoft\Active Setup\Installed Components\`,
		},
	}

	// hiveCanonical maps full root key names back to the "HKXX:" drive form
	// this port returns from ConvertADTRegistryPath.
	hiveCanonical = map[string]string{
		"HKEY_LOCAL_MACHINE":    "HKLM:",
		"HKEY_CLASSES_ROOT":     "HKCR:",
		"HKEY_CURRENT_USER":     "HKCU:",
		"HKEY_USERS":            "HKU:",
		"HKEY_CURRENT_CONFIG":   "HKCC:",
		"HKEY_PERFORMANCE_DATA": "HKPD:",
	}
)

// ConvertADTRegistryPath is the Go port of Convert-ADTRegistryPath: it
// normalizes a registry key path (drive-style "HKLM:\...", bare "HKLM\..."
// or full "HKEY_LOCAL_MACHINE\..." forms, with or without a PowerShell
// provider prefix) and, when wow6432Node is set, rewrites the path for the
// 32-bit registry view using PSADT's WOW64 replacement table.
//
// Deviations from PSADT: the canonical "HKLM:\..." drive form is returned
// instead of the "Microsoft.PowerShell.Core\Registry::HKEY_LOCAL_MACHINE\..."
// provider form (Go has no PowerShell provider); provider prefixes are
// stripped before normalization so WOW64 rewriting applies uniformly; and
// the rewrite is applied whenever wow6432Node is set rather than being
// gated on the process being 64-bit.
func ConvertADTRegistryPath(key string, wow6432Node bool) (string, error) {
	if strings.TrimSpace(key) == "" {
		return "", winerr.Wrap("adt: registry key path is empty", winerr.ErrInvalidOption)
	}

	// Strip any provider prefix ("Microsoft.PowerShell.Core\Registry::",
	// "Registry::", or any partial provider path ending in "::").
	p := key
	if i := strings.LastIndex(p, "::"); i >= 0 {
		p = p[i+2:]
	}

	// Expand drive-style hive abbreviations, matching PSADT's PathMatches
	// separators ":\", ":" and "\" (and end of string).
	upper := strings.ToUpper(p)
	for _, h := range hiveAbbreviations {
		if !strings.HasPrefix(upper, h.abbrev) {
			continue
		}
		rest := p[len(h.abbrev):]
		switch {
		case rest == "":
			p = h.full
		case strings.HasPrefix(rest, `:\`):
			p = h.full + `\` + rest[2:]
		case rest[0] == ':' || rest[0] == '\\':
			p = h.full + `\` + rest[1:]
		default:
			continue // e.g. "HKLMX\..." is not this hive
		}
		break
	}

	// Rewrite for the 32-bit registry view; first matching rule wins (PSADT
	// applies every matching rule in sequence, which can double-insert
	// Wow6432Node for HKLM\SOFTWARE\Classes special keys; this port applies
	// only the most specific rule).
	if wow6432Node {
		for _, r := range wow64Replacements {
			if r.pattern.MatchString(p) {
				p = r.pattern.ReplaceAllString(p, r.replace)
				break
			}
		}
	}

	// Validate the hive and return the canonical drive form.
	root, rest, _ := strings.Cut(p, `\`)
	drive, ok := hiveCanonical[strings.ToUpper(root)]
	if !ok {
		return "", winerr.Wrap("adt: unable to detect target registry hive in ["+key+"]", winerr.ErrInvalidOption)
	}
	rest = strings.TrimSuffix(rest, `\`)
	if rest == "" {
		return drive, nil
	}
	return drive + `\` + rest, nil
}

// splitConvertedPath converts a PSADT-style key to canonical form and splits
// it into the hive and subkey the regkey backend expects.
func splitConvertedPath(key string, wow6432Node bool) (canonical, hive, subkey string, err error) {
	canonical, err = ConvertADTRegistryPath(key, wow6432Node)
	if err != nil {
		return "", "", "", err
	}
	hive, subkey, err = regkey.SplitRoot(canonical)
	if err != nil {
		return "", "", "", err
	}
	return canonical, hive, subkey, nil
}

// normalizeValueName maps PSADT's "(Default)" value-name convention onto the
// empty name the registry APIs use for a key's default value.
func normalizeValueName(name string) string {
	if strings.EqualFold(name, "(Default)") {
		return ""
	}
	return name
}

// expandEnvironmentNames expands %NAME% references the way the Windows
// ExpandEnvironmentStrings API does: defined variables are substituted from
// the process environment and undefined references are left intact.
func expandEnvironmentNames(s string) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '%')
		if start < 0 {
			break
		}
		off := strings.IndexByte(s[start+1:], '%')
		if off < 0 {
			break
		}
		end := start + 1 + off
		name := s[start+1 : end]
		if v, ok := os.LookupEnv(name); ok && name != "" {
			b.WriteString(s[:start])
			b.WriteString(v)
		} else {
			b.WriteString(s[:end+1])
		}
		s = s[end+1:]
	}
	b.WriteString(s)
	return b.String()
}

// registryValueData converts a backend value to the facade's return type,
// expanding REG_EXPAND_SZ values unless suppressed.
func registryValueData(v regkey.Value, doNotExpand bool) any {
	if v.Kind == regkey.KindExpandString && !doNotExpand {
		if s, ok := v.Data.(string); ok {
			return expandEnvironmentNames(s)
		}
	}
	return v.Data
}

// GetADTRegistryKeyOptions mirrors the parameters of Get-ADTRegistryKey.
type GetADTRegistryKeyOptions struct {
	// Key is the registry key path in any PSADT-accepted form.
	Key string
	// Name selects a single value; empty returns every value of the key as
	// a map[string]any. Use "(Default)" for the key's default value.
	Name string
	// Wow6432Node reads the 32-bit registry view on 64-bit systems.
	Wow6432Node bool
	// ReturnEmptyKeyIfExists returns an empty map (instead of an error)
	// when the key exists but holds no values.
	ReturnEmptyKeyIfExists bool
	// DoNotExpandEnvironmentNames returns REG_EXPAND_SZ data unexpanded.
	DoNotExpandEnvironmentNames bool
}

// GetADTRegistryKey is the Go port of Get-ADTRegistryKey: it retrieves a
// single value (string, uint32, uint64, []string or []byte) or, when Name is
// empty, all values of the key as a map[string]any.
//
// Deviation from PSADT: where the PowerShell function returns $null for a
// missing key or value, this port returns an error wrapping ErrNotFound
// (match with errors.Is).
func GetADTRegistryKey(ctx context.Context, opts GetADTRegistryKeyOptions) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: %w", err)
	}
	canonical, hive, subkey, err := splitConvertedPath(opts.Key, opts.Wow6432Node)
	if err != nil {
		return nil, err
	}
	backend := registryBackend()
	exists, err := backend.KeyExists(hive, subkey)
	if err != nil {
		return nil, err
	}
	if !exists {
		logToSession("Registry key ["+canonical+"] does not exist.", LogSeverityWarning, "GetADTRegistryKey")
		return nil, winerr.Wrap("adt: registry key "+canonical, winerr.ErrNotFound)
	}
	if opts.Name != "" {
		logToSession(
			"Getting registry key ["+canonical+"] value ["+opts.Name+"].",
			LogSeverityInfo,
			"GetADTRegistryKey",
		)
		v, err := backend.GetValue(hive, subkey, normalizeValueName(opts.Name))
		if err != nil {
			if errors.Is(err, winerr.ErrNotFound) {
				logToSession(
					"Registry key value ["+canonical+"] ["+opts.Name+"] does not exist.",
					LogSeverityInfo,
					"GetADTRegistryKey",
				)
			}
			return nil, err
		}
		return registryValueData(v, opts.DoNotExpandEnvironmentNames), nil
	}
	logToSession("Getting registry key ["+canonical+"] and all property values.", LogSeverityInfo, "GetADTRegistryKey")
	values, err := backend.EnumValues(hive, subkey)
	if err != nil {
		return nil, err
	}
	if len(values) == 0 && !opts.ReturnEmptyKeyIfExists {
		logToSession("No property values found for ["+canonical+"].", LogSeverityInfo, "GetADTRegistryKey")
		return nil, winerr.Wrap("adt: registry key "+canonical+" has no values", winerr.ErrNotFound)
	}
	out := make(map[string]any, len(values))
	for name, v := range values {
		out[name] = registryValueData(v, opts.DoNotExpandEnvironmentNames)
	}
	return out, nil
}

// SetADTRegistryKeyOptions mirrors the parameters of Set-ADTRegistryKey.
type SetADTRegistryKeyOptions struct {
	// Key is the registry key path in any PSADT-accepted form.
	Key string
	// Name is the value to set; empty (with a nil Value) only creates the
	// key. Use "(Default)" for the key's default value.
	Name string
	// Value is the data to write: string, []string, []byte, or any Go
	// integer type (bool becomes a DWord 0/1).
	Value any
	// Type forces the registry value kind; the zero value infers it from
	// Value's Go type.
	Type RegistryValueKind
	// Wow6432Node writes the 32-bit registry view on 64-bit systems.
	Wow6432Node bool
}

// SetADTRegistryKey is the Go port of Set-ADTRegistryKey: it creates the
// registry key path as needed and, when Name (or a Value for the default
// value) is supplied, writes the value.
func SetADTRegistryKey(ctx context.Context, opts SetADTRegistryKeyOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	canonical, hive, subkey, err := splitConvertedPath(opts.Key, opts.Wow6432Node)
	if err != nil {
		return err
	}
	backend := registryBackend()
	exists, err := backend.KeyExists(hive, subkey)
	if err != nil {
		return err
	}
	if !exists {
		logToSession("Creating registry key ["+canonical+"].", LogSeverityInfo, "SetADTRegistryKey")
		if err := backend.CreateKey(hive, subkey); err != nil {
			return err
		}
	}
	if opts.Name == "" && opts.Value == nil {
		return nil
	}
	value, err := coerceRegistryValue(opts.Value, opts.Type)
	if err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf("Setting registry key value: [%s] [%s = %v].", canonical, opts.Name, opts.Value),
		LogSeverityInfo,
		"SetADTRegistryKey",
	)
	return backend.SetValue(hive, subkey, normalizeValueName(opts.Name), value)
}

// coerceRegistryValue maps a Go value (and optional explicit kind) onto the
// backend's typed registry value.
func coerceRegistryValue(value any, kind RegistryValueKind) (regkey.Value, error) {
	if kind == RegistryValueKindInferred {
		kind = inferRegistryValueKind(value)
	}
	switch kind {
	case RegistryValueKindString, RegistryValueKindExpandString:
		s, ok := value.(string)
		if !ok {
			return regkey.Value{}, winerr.Wrap("adt: registry string value requires a Go string", winerr.ErrInvalidOption)
		}
		k := regkey.KindString
		if kind == RegistryValueKindExpandString {
			k = regkey.KindExpandString
		}
		return regkey.Value{Kind: k, Data: s}, nil
	case RegistryValueKindMultiString:
		s, ok := value.([]string)
		if !ok {
			return regkey.Value{}, winerr.Wrap(
				"adt: registry multi-string value requires a Go []string",
				winerr.ErrInvalidOption,
			)
		}
		return regkey.Value{Kind: regkey.KindMultiString, Data: s}, nil
	case RegistryValueKindBinary:
		b, ok := value.([]byte)
		if !ok {
			return regkey.Value{}, winerr.Wrap("adt: registry binary value requires a Go []byte", winerr.ErrInvalidOption)
		}
		return regkey.Value{Kind: regkey.KindBinary, Data: b}, nil
	case RegistryValueKindDWord:
		u, err := toUint64(value)
		if err != nil {
			return regkey.Value{}, err
		}
		if u > math.MaxUint32 {
			return regkey.Value{}, winerr.Wrap("adt: registry DWord value overflows 32 bits", winerr.ErrInvalidOption)
		}
		return regkey.Value{
			Kind: regkey.KindDWord,
			Data: uint32(u),
		}, nil
	case RegistryValueKindQWord:
		u, err := toUint64(value)
		if err != nil {
			return regkey.Value{}, err
		}
		return regkey.Value{Kind: regkey.KindQWord, Data: u}, nil
	default:
		return regkey.Value{}, winerr.Wrap("adt: unsupported registry value type", winerr.ErrInvalidOption)
	}
}

// inferRegistryValueKind maps a Go type onto the registry kind PSADT would
// pick: strings become REG_SZ, []string REG_MULTI_SZ, []byte REG_BINARY,
// 32-bit-or-smaller integers (and bool) REG_DWORD and 64-bit integers
// REG_QWORD.
func inferRegistryValueKind(value any) RegistryValueKind {
	switch value.(type) {
	case string:
		return RegistryValueKindString
	case []string:
		return RegistryValueKindMultiString
	case []byte:
		return RegistryValueKindBinary
	case int64, uint64:
		return RegistryValueKindQWord
	case bool, int, int8, int16, int32, uint, uint8, uint16, uint32:
		return RegistryValueKindDWord
	default:
		return RegistryValueKindString
	}
}

// toUint64 converts the supported Go integer types (and bool) to uint64.
func toUint64(value any) (uint64, error) {
	switch v := value.(type) {
	case bool:
		if v {
			return 1, nil
		}
		return 0, nil
	case int:
		return uint64(v), nil //#nosec G115 -- negative values wrap to their two's-complement registry representation
	case int8:
		return uint64(v), nil //#nosec G115 -- see above
	case int16:
		return uint64(v), nil //#nosec G115 -- see above
	case int32:
		return uint64(v), nil //#nosec G115 -- see above
	case int64:
		return uint64(v), nil //#nosec G115 -- see above
	case uint:
		return uint64(v), nil
	case uint8:
		return uint64(v), nil
	case uint16:
		return uint64(v), nil
	case uint32:
		return uint64(v), nil
	case uint64:
		return v, nil
	default:
		return 0, winerr.Wrap("adt: registry integer value requires a Go integer type", winerr.ErrInvalidOption)
	}
}

// RemoveADTRegistryKeyOptions mirrors the parameters of Remove-ADTRegistryKey.
type RemoveADTRegistryKeyOptions struct {
	// Key is the registry key path in any PSADT-accepted form.
	Key string
	// Name selects a value to delete; empty deletes the key itself. Use
	// "(Default)" for the key's default value.
	Name string
	// Recurse deletes the key's subtree along with the key.
	Recurse bool
}

// RemoveADTRegistryKey is the Go port of Remove-ADTRegistryKey: it deletes a
// registry value when Name is set, otherwise the key itself (with Recurse
// required when the key has subkeys). As in PSADT, deleting a key or value
// under a key that does not exist logs a warning and succeeds.
func RemoveADTRegistryKey(ctx context.Context, opts RemoveADTRegistryKeyOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	canonical, hive, subkey, err := splitConvertedPath(opts.Key, false)
	if err != nil {
		return err
	}
	backend := registryBackend()
	exists, err := backend.KeyExists(hive, subkey)
	if err != nil {
		return err
	}
	if !exists {
		logToSession(
			"Unable to delete registry key ["+canonical+"] because it does not exist.",
			LogSeverityWarning,
			"RemoveADTRegistryKey",
		)
		return nil
	}
	if opts.Name == "" {
		if !opts.Recurse {
			subs, err := backend.EnumSubkeys(hive, subkey)
			if err != nil {
				return err
			}
			if len(subs) > 0 {
				return winerr.Wrap(
					"adt: unable to delete child key(s) of ["+canonical+"] without Recurse",
					winerr.ErrInvalidOption,
				)
			}
		}
		logToSession("Deleting registry key ["+canonical+"].", LogSeverityInfo, "RemoveADTRegistryKey")
		return backend.DeleteKey(hive, subkey, opts.Recurse)
	}
	logToSession("Deleting registry value ["+canonical+"] ["+opts.Name+"].", LogSeverityInfo, "RemoveADTRegistryKey")
	return backend.DeleteValue(hive, subkey, normalizeValueName(opts.Name))
}

// TestADTRegistryValueOptions mirrors the parameters of Test-ADTRegistryValue.
type TestADTRegistryValueOptions struct {
	// Key is the registry key path in any PSADT-accepted form.
	Key string
	// Name is the value name to test. Use "(Default)" for the default value.
	Name string
	// Wow6432Node tests the 32-bit registry view on 64-bit systems.
	Wow6432Node bool
}

// TestADTRegistryValue is the Go port of Test-ADTRegistryValue: it reports
// whether a registry value exists (a missing key simply reports false).
func TestADTRegistryValue(ctx context.Context, opts TestADTRegistryValueOptions) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("adt: %w", err)
	}
	canonical, hive, subkey, err := splitConvertedPath(opts.Key, opts.Wow6432Node)
	if err != nil {
		return false, err
	}
	_, err = registryBackend().GetValue(hive, subkey, normalizeValueName(opts.Name))
	if err != nil {
		if errors.Is(err, winerr.ErrNotFound) {
			logToSession(
				"Registry key value ["+canonical+"] ["+opts.Name+"] does not exist.",
				LogSeverityInfo,
				"TestADTRegistryValue",
			)
			return false, nil
		}
		return false, err
	}
	logToSession(
		"Registry key value ["+canonical+"] ["+opts.Name+"] does exist.",
		LogSeverityInfo,
		"TestADTRegistryValue",
	)
	return true, nil
}

// hiveLoader mounts and unmounts NTUSER.DAT hives under HKEY_USERS
// (nativeHiveLoader on Windows, stubHiveLoader elsewhere, fakes in tests).
type hiveLoader interface {
	Load(mountKey, hiveFile string) error
	Unload(mountKey string) error
}

// InvokeADTAllUsersRegistryActionOptions mirrors the parameters of
// Invoke-ADTAllUsersRegistryAction.
type InvokeADTAllUsersRegistryActionOptions struct {
	// SkipUnloadedProfiles only visits hives already loaded under
	// HKEY_USERS instead of loading each logged-off profile's NTUSER.DAT.
	SkipUnloadedProfiles bool
	// UserProfiles overrides the profile list (default: GetADTUserProfiles
	// with system and service profiles excluded).
	UserProfiles []UserProfile
}

// InvokeADTAllUsersRegistryAction is the Go port of
// Invoke-ADTAllUsersRegistryAction: it invokes the action once per user
// profile so the action can apply per-user (HKU\<SID>\...) registry changes.
// Profiles whose hive is not currently loaded get their NTUSER.DAT mounted
// under HKEY_USERS for the duration of the action and unloaded afterwards
// (unless SkipUnloadedProfiles). Errors from individual users are logged and
// collected; the remaining users are still visited.
func InvokeADTAllUsersRegistryAction(
	ctx context.Context,
	opts InvokeADTAllUsersRegistryActionOptions,
	action func(ctx context.Context, profile UserProfile) error,
) error {
	if action == nil {
		return winerr.Wrap("adt: all-users registry action is nil", winerr.ErrInvalidOption)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	profiles := opts.UserProfiles
	if profiles == nil {
		var err error
		profiles, err = GetADTUserProfiles(ctx, GetADTUserProfilesOptions{ExcludeDefaultUser: true})
		if err != nil {
			return err
		}
	}
	return invokeAllUsersRegistry(ctx, opts, action, registryBackend(), profiles, defaultHiveLoader())
}

// invokeAllUsersRegistry is the testable orchestration behind
// InvokeADTAllUsersRegistryAction.
func invokeAllUsersRegistry(
	ctx context.Context,
	opts InvokeADTAllUsersRegistryActionOptions,
	action func(ctx context.Context, profile UserProfile) error,
	backend regkey.Backend,
	profiles []UserProfile,
	loader hiveLoader,
) error {
	loadedSIDs, err := backend.EnumSubkeys("HKU", "")
	if err != nil {
		return err
	}
	loaded := make(map[string]bool, len(loadedSIDs))
	for _, sid := range loadedSIDs {
		loaded[strings.ToUpper(sid)] = true
	}

	var errs []error
	for _, profile := range profiles {
		if profile.SID == "" {
			continue
		}
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("adt: %w", err))
			break
		}
		runAction := func() error {
			logToSession(
				"Executing action to modify HKCU registry settings for ["+profile.NTAccount+"].",
				LogSeverityInfo,
				"InvokeADTAllUsersRegistryAction",
			)
			return action(ctx, profile)
		}
		var actionErr error
		switch {
		case loaded[strings.ToUpper(profile.SID)]:
			actionErr = runAction()
		case opts.SkipUnloadedProfiles:
			logToSession(
				"Skipping user ["+profile.NTAccount+"]: hive not loaded and SkipUnloadedProfiles is set.",
				LogSeverityInfo,
				"InvokeADTAllUsersRegistryAction",
			)
			continue
		default:
			hiveFile := filepath.Join(profile.ProfilePath, "NTUSER.DAT")
			logToSession(
				"Loading the registry hive ["+hiveFile+"] for user ["+profile.NTAccount+"].",
				LogSeverityInfo,
				"InvokeADTAllUsersRegistryAction",
			)
			if err := loader.Load(profile.SID, hiveFile); err != nil {
				actionErr = fmt.Errorf("adt: loading hive for %s: %w", profile.NTAccount, err)
				break
			}
			actionErr = runAction()
			// Always unload: a hive left mounted blocks the user's next logon.
			if err := loader.Unload(profile.SID); err != nil {
				logToSession(
					"Failed to unload the registry hive for user ["+profile.NTAccount+"]: "+err.Error(),
					LogSeverityError,
					"InvokeADTAllUsersRegistryAction",
				)
				errs = append(errs, fmt.Errorf("adt: unloading hive for %s: %w", profile.NTAccount, err))
			}
		}
		if actionErr != nil {
			logToSession(
				"Failed to modify the registry hive for user ["+profile.NTAccount+"]: "+actionErr.Error(),
				LogSeverityError,
				"InvokeADTAllUsersRegistryAction",
			)
			errs = append(errs, fmt.Errorf("adt: all-users registry action for %s: %w", profile.SID, actionErr))
		}
	}
	return errors.Join(errs...)
}
