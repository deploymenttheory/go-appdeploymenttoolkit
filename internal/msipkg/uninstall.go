package msipkg

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// InstalledApplication mirrors the fields of PSADT's
// PSADT.AppManagement.InstalledApplication that the Go port consumes.
type InstalledApplication struct {
	// DisplayName is the application's ARP display name.
	DisplayName string
	// DisplayVersion is the ARP display version ("" when absent).
	DisplayVersion string
	// Publisher is the ARP publisher ("" when absent).
	Publisher string
	// UninstallString is the raw ARP UninstallString ("" when absent).
	UninstallString string
	// QuietUninstallString is the raw ARP QuietUninstallString ("" when absent).
	QuietUninstallString string
	// ProductCode is the canonical "{GUID}" for Windows Installer products
	// whose uninstall key name is a product code, otherwise "".
	ProductCode string
	// WindowsInstaller reports whether the entry is serviced by msiexec
	// (WindowsInstaller registry flag or an msiexec uninstall string).
	WindowsInstaller bool
	// SystemComponent mirrors the ARP SystemComponent flag.
	SystemComponent bool
	// Is64Bit reports whether the entry came from the 64-bit registry view
	// (always false for per-user HKCU entries, where PSADT reports null).
	Is64Bit bool
}

// FindOptions mirrors the filtering parameters of Get-ADTApplication.
type FindOptions struct {
	// Names filters by display name; empty matches every application.
	Names []string
	// NameMatch is Contains (default, ""), Exact, Wildcard or Regex.
	NameMatch string
	// ProductCodes filters by MSI product code (any GUID format).
	ProductCodes []string
	// ApplicationType is All (default, ""), MSI or EXE.
	ApplicationType string
	// IncludeUpdatesAndHotfixes includes Microsoft update/hotfix entries.
	IncludeUpdatesAndHotfixes bool
}

// updatesAndHotfixesRegex ports Get-ADTApplication's update/hotfix filter.
var updatesAndHotfixesRegex = regexp.MustCompile(`(?i)kb\d+|(Cumulative|Security) Update|Hotfix`)

// guidRegex matches a GUID with or without surrounding braces.
var guidRegex = regexp.MustCompile(`^\{?[0-9A-Fa-f]{8}-(?:[0-9A-Fa-f]{4}-){3}[0-9A-Fa-f]{12}\}?$`)

// IsProductCode reports whether s is a Windows Installer product code GUID
// (with or without braces).
func IsProductCode(s string) bool {
	return guidRegex.MatchString(s)
}

// NormalizeProductCode canonicalizes a GUID to the uppercase "{...}" form
// msiexec and the uninstall registry use.
func NormalizeProductCode(s string) (string, error) {
	if !IsProductCode(s) {
		return "", winerr.Wrap("msipkg: "+s+" is not a product code GUID", winerr.ErrInvalidOption)
	}
	return "{" + strings.ToUpper(strings.Trim(s, "{}")) + "}", nil
}

// uninstallRoot is one of the three ARP registry locations.
type uninstallRoot struct {
	hive  string
	path  string
	hklm  bool
	wow64 bool
}

// uninstallRoots returns the ARP roots in PSADT's enumeration order,
// including the 32-bit view only for 64-bit processes.
func uninstallRoots() []uninstallRoot {
	roots := []uninstallRoot{
		{hive: "HKCU", path: `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{hive: "HKLM", path: `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`, hklm: true},
	}
	if enumerateWowNode() {
		roots = append(roots, uninstallRoot{
			hive:  "HKLM",
			path:  `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`,
			hklm:  true,
			wow64: true,
		})
	}
	return roots
}

// FindInstalledApplications is the enumeration engine behind
// Get-ADTApplication: it walks the per-user and per-machine uninstall keys of
// the supplied registry backend and returns the entries passing the filters.
func FindInstalledApplications(
	backend regkey.Backend,
	opts FindOptions,
) ([]InstalledApplication, error) {
	nameMatcher, err := newNameMatcher(opts.Names, opts.NameMatch)
	if err != nil {
		return nil, err
	}
	productCodes, err := normalizeProductCodes(opts.ProductCodes)
	if err != nil {
		return nil, err
	}
	appType := opts.ApplicationType
	if appType == "" {
		appType = "All"
	}
	if !strings.EqualFold(appType, "All") && !strings.EqualFold(appType, "MSI") &&
		!strings.EqualFold(appType, "EXE") {
		return nil, winerr.Wrap(
			"msipkg: ApplicationType "+opts.ApplicationType,
			winerr.ErrInvalidOption,
		)
	}

	var apps []InstalledApplication
	for _, root := range uninstallRoots() {
		subkeys, err := backend.EnumSubkeys(root.hive, root.path)
		if err != nil {
			// Missing roots are ignored, mirroring -ErrorAction Ignore.
			continue
		}
		for _, sub := range subkeys {
			values, err := backend.EnumValues(root.hive, root.path+`\`+sub)
			if err != nil {
				continue
			}
			app, ok := buildInstalledApplication(root, sub, values, opts.IncludeUpdatesAndHotfixes)
			if !ok {
				continue
			}
			if !nameMatcher(app.DisplayName) {
				continue
			}
			if (strings.EqualFold(appType, "MSI") && !app.WindowsInstaller) ||
				(strings.EqualFold(appType, "EXE") && app.WindowsInstaller) {
				continue
			}
			if len(productCodes) > 0 && !containsFold(productCodes, app.ProductCode) {
				continue
			}
			apps = append(apps, app)
		}
	}
	return apps, nil
}

// buildInstalledApplication converts one uninstall key's values into an
// InstalledApplication, reporting !ok for entries PSADT skips (no values, no
// display name, updates/hotfixes).
func buildInstalledApplication(
	root uninstallRoot,
	keyName string,
	values map[string]regkey.Value,
	includeUpdates bool,
) (InstalledApplication, bool) {
	if len(values) == 0 {
		return InstalledApplication{}, false
	}
	displayName := stringValue(values, "DisplayName")
	if strings.TrimSpace(displayName) == "" {
		return InstalledApplication{}, false
	}
	if !includeUpdates && updatesAndHotfixesRegex.MatchString(displayName) {
		return InstalledApplication{}, false
	}
	uninstallString := stringValue(values, "UninstallString")
	quietUninstallString := stringValue(values, "QuietUninstallString")
	windowsInstaller := boolValue(values, "WindowsInstaller") ||
		strings.Contains(strings.ToLower(uninstallString), "msiexec") ||
		strings.Contains(strings.ToLower(quietUninstallString), "msiexec")
	productCode := ""
	if windowsInstaller {
		if code, err := NormalizeProductCode(keyName); err == nil {
			productCode = code
		}
	}
	return InstalledApplication{
		DisplayName:          displayName,
		DisplayVersion:       stringValue(values, "DisplayVersion"),
		Publisher:            stringValue(values, "Publisher"),
		UninstallString:      uninstallString,
		QuietUninstallString: quietUninstallString,
		ProductCode:          productCode,
		WindowsInstaller:     windowsInstaller,
		SystemComponent:      boolValue(values, "SystemComponent"),
		Is64Bit:              root.hklm && !root.wow64 && enumerateWowNode(),
	}, true
}

// newNameMatcher compiles the Get-ADTApplication name filter: Contains
// (default), Exact, Wildcard or Regex, all case-insensitive like PowerShell.
func newNameMatcher(names []string, mode string) (func(string) bool, error) {
	if len(names) == 0 {
		return func(string) bool { return true }, nil
	}
	if mode == "" {
		mode = "Contains"
	}
	switch {
	case strings.EqualFold(mode, "Contains"):
		return func(displayName string) bool {
			lower := strings.ToLower(displayName)
			for _, name := range names {
				if strings.Contains(lower, strings.ToLower(name)) {
					return true
				}
			}
			return false
		}, nil
	case strings.EqualFold(mode, "Exact"):
		return func(displayName string) bool {
			return containsFold(names, displayName)
		}, nil
	case strings.EqualFold(mode, "Wildcard"):
		patterns := make([]*regexp.Regexp, 0, len(names))
		for _, name := range names {
			re, err := regexp.Compile(wildcardToRegex(name))
			if err != nil {
				return nil, fmt.Errorf("msipkg: compiling wildcard %q: %w", name, err)
			}
			patterns = append(patterns, re)
		}
		return func(displayName string) bool {
			for _, re := range patterns {
				if re.MatchString(displayName) {
					return true
				}
			}
			return false
		}, nil
	case strings.EqualFold(mode, "Regex"):
		patterns := make([]*regexp.Regexp, 0, len(names))
		for _, name := range names {
			re, err := regexp.Compile("(?i)" + name)
			if err != nil {
				return nil, fmt.Errorf("msipkg: compiling regex %q: %w", name, err)
			}
			patterns = append(patterns, re)
		}
		return func(displayName string) bool {
			for _, re := range patterns {
				if re.MatchString(displayName) {
					return true
				}
			}
			return false
		}, nil
	default:
		return nil, winerr.Wrap("msipkg: NameMatch "+mode, winerr.ErrInvalidOption)
	}
}

// wildcardToRegex converts a PowerShell -like pattern ("*"/"?") to an
// anchored case-insensitive regular expression.
func wildcardToRegex(pattern string) string {
	var b strings.Builder
	b.WriteString("(?i)^")
	for _, r := range pattern {
		switch r {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteString(".")
		default:
			b.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	b.WriteString("$")
	return b.String()
}

// normalizeProductCodes canonicalizes the ProductCode filter values.
func normalizeProductCodes(codes []string) ([]string, error) {
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		normalized, err := NormalizeProductCode(code)
		if err != nil {
			return nil, err
		}
		out = append(out, normalized)
	}
	return out, nil
}

// containsFold reports case-insensitive membership.
func containsFold(list []string, s string) bool {
	for _, item := range list {
		if strings.EqualFold(item, s) {
			return true
		}
	}
	return false
}

// stringValue extracts a string registry value, returning "" for missing or
// non-string data.
func stringValue(values map[string]regkey.Value, name string) string {
	v, ok := values[name]
	if !ok {
		return ""
	}
	if s, ok := v.Data.(string); ok {
		return s
	}
	return ""
}

// boolValue reports whether a DWORD-style registry value is present and
// non-zero.
func boolValue(values map[string]regkey.Value, name string) bool {
	v, ok := values[name]
	if !ok {
		return false
	}
	switch data := v.Data.(type) {
	case uint32:
		return data != 0
	case uint64:
		return data != 0
	default:
		return false
	}
}
