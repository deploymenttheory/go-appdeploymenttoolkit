package winadt

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/inifile"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// INI facade functions are backed by the pure-Go internal/inifile package
// rather than the Win32 GetPrivateProfileString family PSADT uses; behavior
// is compatible for well-formed files and works on every platform.

// IniValueOptions mirrors the parameters of Get-ADTIniValue and
// Remove-ADTIniValue.
type IniValueOptions struct {
	// FilePath is the path to the INI file.
	FilePath string
	// Section is the section within the INI file.
	Section string
	// Key is the key within the section.
	Key string
}

// SetADTIniValueOptions mirrors the parameters of Set-ADTIniValue.
type SetADTIniValueOptions struct {
	// FilePath is the path to the INI file; it is created if missing.
	FilePath string
	// Section is the section within the INI file; it is created if missing.
	Section string
	// Key is the key within the section.
	Key string
	// Value is the value to write (empty writes "Key=").
	Value string
}

// IniSectionOptions mirrors the parameters of Get-ADTIniSection,
// Set-ADTIniSection and Remove-ADTIniSection. Content and Overwrite are
// only consulted by SetADTIniSection.
type IniSectionOptions struct {
	// FilePath is the path to the INI file.
	FilePath string
	// Section is the section within the INI file.
	Section string
	// Content is the key/value content written by SetADTIniSection.
	Content map[string]string
	// Overwrite replaces the whole section instead of merging Content into
	// the section's existing keys.
	Overwrite bool
}

// validateIniArgs enforces PSADT's ValidateNotNullOrWhiteSpace checks on the
// mandatory INI parameters.
func validateIniArgs(args map[string]string) error {
	for name, value := range args {
		if strings.TrimSpace(value) == "" {
			return winerr.Wrap("adt: INI parameter "+name+" is empty", winerr.ErrInvalidOption)
		}
	}
	return nil
}

// GetADTIniValue is the Go port of Get-ADTIniValue: it reads a value from a
// section of an INI file. The error wraps ErrNotFound when the file, the
// section or the key does not exist.
func GetADTIniValue(ctx context.Context, opts IniValueOptions) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
		"Key":      opts.Key,
	}); err != nil {
		return "", err
	}
	logToSession(
		fmt.Sprintf("Reading INI value: [FilePath = %s] [Section = %s] [Key = %s].", opts.FilePath, opts.Section, opts.Key),
		LogSeverityInfo,
		"GetADTIniValue",
	)
	value, err := inifile.ReadValue(opts.FilePath, opts.Section, opts.Key)
	if err != nil {
		return "", err
	}
	logToSession("INI value: ["+value+"].", LogSeverityInfo, "GetADTIniValue")
	return value, nil
}

// SetADTIniValue is the Go port of Set-ADTIniValue: it writes a value to a
// section of an INI file, creating the file and the section as needed.
func SetADTIniValue(ctx context.Context, opts SetADTIniValueOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
		"Key":      opts.Key,
	}); err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf(
			"Writing INI value: [FilePath = %s] [Section = %s] [Key = %s] [Value = %s].",
			opts.FilePath, opts.Section, opts.Key, opts.Value,
		),
		LogSeverityInfo,
		"SetADTIniValue",
	)
	return inifile.WriteValue(opts.FilePath, opts.Section, opts.Key, opts.Value)
}

// RemoveADTIniValue is the Go port of Remove-ADTIniValue: it removes a key
// from a section of an INI file. As in PSADT, removing a key that does not
// exist is not an error.
func RemoveADTIniValue(ctx context.Context, opts IniValueOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
		"Key":      opts.Key,
	}); err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf(
			"Removing INI value: [FilePath = %s] [Section = %s] [Key = %s].",
			opts.FilePath, opts.Section, opts.Key,
		),
		LogSeverityInfo,
		"RemoveADTIniValue",
	)
	return inifile.DeleteValue(opts.FilePath, opts.Section, opts.Key)
}

// GetADTIniSection is the Go port of Get-ADTIniSection: it reads an entire
// section of an INI file as a key/value map. The error wraps ErrNotFound
// when the file or the section does not exist.
func GetADTIniSection(ctx context.Context, opts IniSectionOptions) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
	}); err != nil {
		return nil, err
	}
	logToSession(
		fmt.Sprintf("Reading INI section: [FilePath = %s] [Section = %s].", opts.FilePath, opts.Section),
		LogSeverityInfo,
		"GetADTIniSection",
	)
	return inifile.ReadSection(opts.FilePath, opts.Section)
}

// SetADTIniSection is the Go port of Set-ADTIniSection: it writes a section
// of an INI file, creating the file and the section as needed. By default
// Content is merged over the section's existing keys; Overwrite replaces
// the entire section. As in PSADT, merging empty Content is a no-op while
// overwriting with empty Content empties the section.
func SetADTIniSection(ctx context.Context, opts IniSectionOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
	}); err != nil {
		return err
	}
	content := opts.Content
	if !opts.Overwrite {
		if len(content) == 0 {
			logToSession(
				fmt.Sprintf(
					"No content provided to write to INI section: [FilePath = %s] [Section = %s].",
					opts.FilePath, opts.Section,
				),
				LogSeverityInfo,
				"SetADTIniSection",
			)
			return nil
		}
		existing, err := inifile.ReadSection(opts.FilePath, opts.Section)
		if err != nil && !errors.Is(err, winerr.ErrNotFound) {
			return err
		}
		merged := make(map[string]string, len(existing)+len(content))
		for k, v := range existing {
			merged[k] = v
		}
		for k, v := range content {
			merged[k] = v
		}
		content = merged
	}
	verb := "Writing"
	if opts.Overwrite {
		verb = "Overwriting"
	}
	logToSession(
		fmt.Sprintf("%s INI section: [FilePath = %s] [Section = %s].", verb, opts.FilePath, opts.Section),
		LogSeverityInfo,
		"SetADTIniSection",
	)
	return inifile.WriteSection(opts.FilePath, opts.Section, content)
}

// RemoveADTIniSection is the Go port of Remove-ADTIniSection: it removes an
// entire section from an INI file. As in PSADT, removing a section that
// does not exist is not an error.
func RemoveADTIniSection(ctx context.Context, opts IniSectionOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if err := validateIniArgs(map[string]string{
		"FilePath": opts.FilePath,
		"Section":  opts.Section,
	}); err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf("Removing INI section: [FilePath = %s] [Section = %s].", opts.FilePath, opts.Section),
		LogSeverityInfo,
		"RemoveADTIniSection",
	)
	return inifile.DeleteSection(opts.FilePath, opts.Section)
}
