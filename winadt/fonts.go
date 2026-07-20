package winadt

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// fontsRegistryKey is the machine-wide registered-fonts key that both
// Add-ADTFont and Remove-ADTFont manipulate.
const fontsRegistryKey = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts`

// fontTypeSuffixes maps a supported font extension onto the registry
// display-name suffix PSADT appends (Add-ADTFont's $fontTypes table).
var fontTypeSuffixes = map[string]string{
	".ttf": " (TrueType)",
	".ttc": " (TrueType)",
	".otf": " (OpenType)",
}

// AddADTFontOptions mirrors the parameters of Add-ADTFont.
type AddADTFontOptions struct {
	// FilePath is the .ttf, .ttc or .otf font file to install.
	FilePath string
}

// RemoveADTFontOptions mirrors the parameters of Remove-ADTFont.
type RemoveADTFontOptions struct {
	// Name is either the font file name (e.g. "arial.ttf") or the font's
	// registry display name (e.g. "Arial (TrueType)").
	Name string
}

// AddADTFont is the Go port of Add-ADTFont: it installs a font file by copying
// it into the system Fonts directory, registering the font resource (and
// broadcasting WM_FONTCHANGE), and recording it under
// HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Fonts.
//
// Deviation from PSADT: this port installs a single file (Add-ADTFont accepts
// wildcards and directory recursion); callers iterate directories themselves.
func AddADTFont(ctx context.Context, opts AddADTFontOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: AddADTFont: %w", err)
	}
	if strings.TrimSpace(opts.FilePath) == "" {
		return fmt.Errorf("adt: FilePath is required: %w", ErrInvalidOption)
	}
	ext := strings.ToLower(filepath.Ext(opts.FilePath))
	suffix, ok := fontTypeSuffixes[ext]
	if !ok {
		return fmt.Errorf("adt: file [%s] is not a supported font type: %w",
			filepath.Base(opts.FilePath), ErrInvalidOption)
	}
	fontsDir, err := systemFontsDir()
	if err != nil {
		return err
	}
	fileName := filepath.Base(opts.FilePath)
	destPath := filepath.Join(fontsDir, fileName)
	logToSession(fmt.Sprintf("Installing font [%s]...", fileName), LogSeverityInfo, "AddADTFont")

	if _, statErr := os.Stat(destPath); errors.Is(statErr, os.ErrNotExist) {
		if err := copyFontFile(opts.FilePath, destPath); err != nil {
			return err
		}
	}

	if err := addFontResource(destPath); err != nil {
		return err
	}

	regName := fontRegistryName(destPath, suffix)
	if err := SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
		Key:   fontsRegistryKey,
		Name:  regName,
		Value: fileName,
	}); err != nil {
		return err
	}
	logToSession(fmt.Sprintf("Successfully installed font [%s] as [%s].", fileName, regName),
		LogSeveritySuccess, "AddADTFont")
	return nil
}

// RemoveADTFont is the Go port of Remove-ADTFont: it removes a font by name
// (font file name or registry display name), unregistering the font resource
// (broadcasting WM_FONTCHANGE), deleting its registry entry and removing the
// file from the system Fonts directory.
func RemoveADTFont(ctx context.Context, opts RemoveADTFontOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: RemoveADTFont: %w", err)
	}
	if strings.TrimSpace(opts.Name) == "" {
		return fmt.Errorf("adt: Name is required: %w", ErrInvalidOption)
	}
	fontsDir, err := systemFontsDir()
	if err != nil {
		return err
	}
	logToSession(fmt.Sprintf("Removing font [%s]...", opts.Name), LogSeverityInfo, "RemoveADTFont")

	values, err := fontsRegistryValues(ctx)
	if err != nil {
		return err
	}
	displayName, fileName := resolveFontRemoval(opts.Name, values, fontsDir)
	if displayName == "" && fileName == "" {
		logToSession(fmt.Sprintf("The font [%s] is already uninstalled.", opts.Name),
			LogSeverityInfo, "RemoveADTFont")
		return nil
	}

	if fileName != "" {
		fontFilePath := filepath.Join(fontsDir, fileName)
		if info, statErr := os.Stat(fontFilePath); statErr == nil && !info.IsDir() {
			if err := removeFontResource(fontFilePath); err != nil {
				return err
			}
			if err := os.Remove(fontFilePath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("adt: removing font file: %w", err)
			}
		}
	}
	if displayName != "" {
		if err := RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{
			Key:  fontsRegistryKey,
			Name: displayName,
		}); err != nil {
			return err
		}
	}
	logToSession(fmt.Sprintf("Successfully uninstalled font [%s].", opts.Name),
		LogSeveritySuccess, "RemoveADTFont")
	return nil
}

// fontsRegistryValues reads every value of the registered-fonts key, returning
// an empty map when the key or its values do not exist.
func fontsRegistryValues(ctx context.Context) (map[string]string, error) {
	raw, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{
		Key:                    fontsRegistryKey,
		ReturnEmptyKeyIfExists: true,
	})
	if err != nil {
		if errors.Is(err, winerr.ErrNotFound) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(m))
	for name, v := range m {
		if s, ok := v.(string); ok {
			out[name] = s
		}
	}
	return out, nil
}

// resolveFontRemoval ports Remove-ADTFont's name resolution: it maps the
// supplied name (a registry display name or a font file name) onto the display
// name to delete and the file name to remove. An empty result for both means
// the font is not registered. When the name matches a file present in the
// Fonts directory it is treated as a file name; otherwise as a display name.
func resolveFontRemoval(name string, values map[string]string, fontsDir string) (displayName, fileName string) {
	if fontsDir != "" {
		if info, err := os.Stat(filepath.Join(fontsDir, name)); err == nil && !info.IsDir() {
			// Name is a file name: find the display name whose value matches.
			for dn, fn := range values {
				if strings.EqualFold(fn, name) {
					return dn, name
				}
			}
			return "", name
		}
	}
	// Name is a display name: return its registered file name.
	for dn, fn := range values {
		if strings.EqualFold(dn, name) {
			return dn, fn
		}
	}
	return "", ""
}

// fontRegistryName composes the registry display name PSADT records: the
// font's internal title plus the type suffix, falling back to the file's base
// name (without extension) when the title cannot be read.
func fontRegistryName(path, suffix string) string {
	title := ""
	if data, err := os.ReadFile(path); err == nil { //#nosec G304 -- caller-provided font path being installed
		if t, err := fontTitle(data); err == nil {
			title = t
		}
	}
	if title == "" {
		base := filepath.Base(path)
		title = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return title + suffix
}

// systemFontsDir returns %WINDIR%\Fonts, mirroring Add-ADTFont's use of the
// Windows special folder.
func systemFontsDir() (string, error) {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = os.Getenv("SystemRoot")
	}
	if windir == "" {
		return "", winerr.Wrap("adt: Windows directory unresolved", winerr.ErrNotFound)
	}
	return filepath.Join(windir, "Fonts"), nil
}

// copyFontFile copies a font file to the destination, mirroring Add-ADTFont's
// Copy-Item -Force.
func copyFontFile(src, dest string) error {
	data, err := os.ReadFile(src) //#nosec G304 -- caller-provided font path being installed
	if err != nil {
		return fmt.Errorf("adt: reading font file: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil { //#nosec G306 -- fonts must be world-readable
		return fmt.Errorf("adt: writing font file: %w", err)
	}
	return nil
}

// sfnt name-table identifiers used by fontTitle.
const (
	sfntTagTTC       = 0x74746366 // 'ttcf', a TrueType collection
	nameIDFontFamily = 1          // font family name
	nameIDFullName   = 4          // full font name
	platformWindows  = 3          // Windows platform (UTF-16BE strings)
	platformMac      = 1          // Macintosh platform (single-byte strings)
)

// fontTitle parses an sfnt (TrueType/OpenType/TTC) font and returns its title:
// the full font name (name ID 4), falling back to the family name (name ID 1).
// It ports the intent of PSADT's FontUtilities.GetFontTitle without any GDI
// dependency, so the logic is unit-testable on any platform.
func fontTitle(data []byte) (string, error) {
	tableOffset, err := sfntTableDirectoryOffset(data)
	if err != nil {
		return "", err
	}
	nameOffset, nameLen, err := sfntFindTable(data, tableOffset, "name")
	if err != nil {
		return "", err
	}
	return parseNameTable(data, nameOffset, nameLen)
}

// sfntTableDirectoryOffset returns the offset of the table directory, handling
// both plain sfnt files and TrueType collections (which point at the first
// contained font).
func sfntTableDirectoryOffset(data []byte) (uint32, error) {
	if len(data) < 12 {
		return 0, winerr.Wrap("adt: font data is truncated", winerr.ErrInvalidOption)
	}
	if binary.BigEndian.Uint32(data) == sfntTagTTC {
		if len(data) < 16 {
			return 0, winerr.Wrap("adt: font collection header is truncated", winerr.ErrInvalidOption)
		}
		if binary.BigEndian.Uint32(data[8:]) == 0 { // numFonts
			return 0, winerr.Wrap("adt: font collection is empty", winerr.ErrInvalidOption)
		}
		return binary.BigEndian.Uint32(data[12:]), nil // first font's table directory
	}
	return 0, nil
}

// sfntFindTable locates a named table (e.g. "name") within the table directory
// beginning at dirOffset, returning its byte offset and length.
func sfntFindTable(data []byte, dirOffset uint32, tag string) (offset, length uint32, err error) {
	if uint64(dirOffset)+6 > uint64(len(data)) {
		return 0, 0, winerr.Wrap("adt: font table directory out of range", winerr.ErrInvalidOption)
	}
	numTables := binary.BigEndian.Uint16(data[dirOffset+4:])
	record := uint64(dirOffset) + 12 // skip sfntVersion + counts
	for i := 0; i < int(numTables); i++ {
		if record+16 > uint64(len(data)) {
			return 0, 0, winerr.Wrap("adt: font table record out of range", winerr.ErrInvalidOption)
		}
		if string(data[record:record+4]) == tag {
			off := binary.BigEndian.Uint32(data[record+8:])
			l := binary.BigEndian.Uint32(data[record+12:])
			return off, l, nil
		}
		record += 16
	}
	return 0, 0, winerr.Wrap("adt: font has no name table", winerr.ErrNotFound)
}

// parseNameTable extracts the preferred title from an sfnt 'name' table.
func parseNameTable(data []byte, offset, length uint32) (string, error) {
	if uint64(offset)+6 > uint64(len(data)) {
		return "", winerr.Wrap("adt: font name table out of range", winerr.ErrInvalidOption)
	}
	count := binary.BigEndian.Uint16(data[offset+2:])
	stringOffset := uint64(offset) + uint64(binary.BigEndian.Uint16(data[offset+4:]))
	recordBase := uint64(offset) + 6

	var family string
	full := ""
	for i := 0; i < int(count); i++ {
		rec := recordBase + uint64(i)*12
		if rec+12 > uint64(len(data)) {
			break
		}
		platformID := binary.BigEndian.Uint16(data[rec:])
		nameID := binary.BigEndian.Uint16(data[rec+6:])
		if nameID != nameIDFullName && nameID != nameIDFontFamily {
			continue
		}
		strLen := uint64(binary.BigEndian.Uint16(data[rec+8:]))
		strOff := stringOffset + uint64(binary.BigEndian.Uint16(data[rec+10:]))
		if strOff+strLen > uint64(len(data)) {
			continue
		}
		value := decodeNameString(data[strOff:strOff+strLen], platformID)
		if value == "" {
			continue
		}
		switch nameID {
		case nameIDFullName:
			if full == "" || platformID == platformWindows {
				full = value
			}
		case nameIDFontFamily:
			if family == "" || platformID == platformWindows {
				family = value
			}
		}
	}
	if full != "" {
		return full, nil
	}
	if family != "" {
		return family, nil
	}
	return "", winerr.Wrap("adt: font name not found", winerr.ErrNotFound)
}

// decodeNameString decodes a name-record string: Windows-platform strings are
// UTF-16BE, other platforms are treated as Latin-1/ASCII.
func decodeNameString(b []byte, platformID uint16) string {
	if platformID == platformWindows {
		if len(b)%2 != 0 {
			b = b[:len(b)-1]
		}
		u := make([]uint16, len(b)/2)
		for i := range u {
			u[i] = binary.BigEndian.Uint16(b[i*2:])
		}
		return strings.TrimSpace(string(utf16.Decode(u)))
	}
	return strings.TrimSpace(string(b))
}
