// Package regkey defines the registry backend seam used by portable toolkit
// logic (deferral history, uninstall-key enumeration, policy reads). The
// Windows implementation lives in backend_windows.go; tests use Fake.
package regkey

import (
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Value kinds mirror registry value types the toolkit reads and writes.
type ValueKind int

// ValueKind values.
const (
	KindString ValueKind = iota
	KindExpandString
	KindDWord
	KindQWord
	KindMultiString
	KindBinary
)

// Value is one registry value: String/ExpandString carry string, DWord
// uint32, QWord uint64, MultiString []string, Binary []byte.
type Value struct {
	Kind ValueKind
	Data any
}

// Backend abstracts the registry operations the toolkit needs. Paths are
// rooted with PSADT-style hive prefixes ("HKLM", "HKCU", "HKU", "HKCR",
// "HKCC") and use backslash separators, e.g. "HKLM", `SOFTWARE\PSAppDeployToolkit`.
type Backend interface {
	// GetValue returns the named value; winerr.ErrNotFound when the key or
	// value does not exist. Name "" addresses the default value.
	GetValue(hive, path, name string) (Value, error)
	// SetValue creates the key path as needed and writes the value.
	SetValue(hive, path, name string, value Value) error
	// DeleteValue removes a value; winerr.ErrNotFound if absent.
	DeleteValue(hive, path, name string) error
	// DeleteKey removes a key and, when recurse is set, its subtree.
	DeleteKey(hive, path string, recurse bool) error
	// CreateKey ensures the key path exists.
	CreateKey(hive, path string) error
	// KeyExists reports whether the key exists.
	KeyExists(hive, path string) (bool, error)
	// EnumValues lists the values of a key.
	EnumValues(hive, path string) (map[string]Value, error)
	// EnumSubkeys lists immediate subkey names.
	EnumSubkeys(hive, path string) ([]string, error)
}

// SplitRoot splits a PSADT/PowerShell-style registry path into hive and
// subkey: accepts "HKLM:\SOFTWARE\X", "HKLM\SOFTWARE\X" and
// "HKEY_LOCAL_MACHINE\SOFTWARE\X" forms.
func SplitRoot(path string) (hive, subkey string, err error) {
	p := strings.TrimPrefix(path, `Microsoft.PowerShell.Core\Registry::`)
	p = strings.TrimPrefix(p, `Registry::`)
	root, rest, _ := strings.Cut(p, `\`)
	root = strings.TrimSuffix(strings.ToUpper(root), ":")
	switch root {
	case "HKLM", "HKEY_LOCAL_MACHINE":
		hive = "HKLM"
	case "HKCU", "HKEY_CURRENT_USER":
		hive = "HKCU"
	case "HKU", "HKEY_USERS":
		hive = "HKU"
	case "HKCR", "HKEY_CLASSES_ROOT":
		hive = "HKCR"
	case "HKCC", "HKEY_CURRENT_CONFIG":
		hive = "HKCC"
	default:
		return "", "", winerr.Wrap("regkey: unrecognized hive "+root, winerr.ErrInvalidOption)
	}
	return hive, rest, nil
}
