package fsops

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FileVersionInfo carries a file's version resources: FileVersion is the
// binary VS_FIXEDFILEINFO version ("a.b.c.d"); ProductVersion is the string
// table's ProductVersion when present (it can differ from the fixed value),
// otherwise the fixed product version.
type FileVersionInfo struct {
	FileVersion    string
	ProductVersion string
}

// GetFileVersion is the engine behind Get-ADTFileVersion: it queries the
// file's version resource via GetFileVersionInfo/VerQueryValue.
func GetFileVersion(path string) (FileVersionInfo, error) {
	var zeroHandle windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zeroHandle)
	if err != nil {
		return FileVersionInfo{}, fmt.Errorf("fsops: GetFileVersionInfoSize %s: %w", path, err)
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return FileVersionInfo{}, fmt.Errorf("fsops: GetFileVersionInfo %s: %w", path, err)
	}

	var fixed *windows.VS_FIXEDFILEINFO
	var fixedLen uint32
	err = windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &fixedLen)
	if err != nil {
		return FileVersionInfo{}, fmt.Errorf("fsops: VerQueryValue(\\) %s: %w", path, err)
	}
	info := FileVersionInfo{
		FileVersion: versionString(fixed.FileVersionMS, fixed.FileVersionLS),
		ProductVersion: versionString(
			fixed.ProductVersionMS,
			fixed.ProductVersionLS,
		),
	}
	if pv := stringFileInfoValue(buf, "ProductVersion"); pv != "" {
		info.ProductVersion = pv
	}
	return info, nil
}

// versionString renders the packed MS/LS version pair as "a.b.c.d".
func versionString(ms, ls uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}

// verLangCodepage mirrors the \VarFileInfo\Translation entry layout.
type verLangCodepage struct {
	Lang     uint16
	Codepage uint16
}

// stringFileInfoValue reads a \StringFileInfo value for the first available
// translation, returning "" when the resource has no string table.
func stringFileInfoValue(block []byte, name string) string {
	var translations *verLangCodepage
	var length uint32
	err := windows.VerQueryValue(
		unsafe.Pointer(&block[0]),
		`\VarFileInfo\Translation`,
		unsafe.Pointer(&translations),
		&length,
	)
	if err != nil || length < uint32(unsafe.Sizeof(verLangCodepage{})) {
		return ""
	}
	count := length / uint32(unsafe.Sizeof(verLangCodepage{}))
	pairs := unsafe.Slice(translations, count)
	for _, lc := range pairs {
		subBlock := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lc.Lang, lc.Codepage, name)
		var value *uint16
		var valueLen uint32
		err := windows.VerQueryValue(
			unsafe.Pointer(&block[0]),
			subBlock,
			unsafe.Pointer(&value),
			&valueLen,
		)
		if err != nil || value == nil || valueLen == 0 {
			continue
		}
		if s := strings.TrimSpace(windows.UTF16PtrToString(value)); s != "" {
			return s
		}
	}
	return ""
}
