//go:build windows

package winadt

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// executableVersionInfo reads the version resource of a PE file via
// GetFileVersionInfo/VerQueryValue, filling the string-table fields plus the
// fixed FileVersion/ProductVersion.
func executableVersionInfo(path string) (ExecutableInfo, error) {
	var zero windows.Handle
	size, err := windows.GetFileVersionInfoSize(path, &zero)
	if err != nil {
		return ExecutableInfo{}, fmt.Errorf("adt: GetFileVersionInfoSize [%s]: %w", path, err)
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ExecutableInfo{}, fmt.Errorf("adt: GetFileVersionInfo [%s]: %w", path, err)
	}

	info := ExecutableInfo{}
	var fixed *windows.VS_FIXEDFILEINFO
	var fixedLen uint32
	if err := windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\`, unsafe.Pointer(&fixed), &fixedLen); err == nil && fixed != nil {
		info.FileVersion = fixedVersionString(fixed.FileVersionMS, fixed.FileVersionLS)
		info.ProductVersion = fixedVersionString(fixed.ProductVersionMS, fixed.ProductVersionLS)
	}

	lang, cp, ok := firstTranslation(buf)
	if !ok {
		return info, nil
	}
	get := func(name string) string { return stringTableValue(buf, lang, cp, name) }
	info.ProductName = get("ProductName")
	info.FileDescription = get("FileDescription")
	info.CompanyName = get("CompanyName")
	info.InternalName = get("InternalName")
	info.OriginalFilename = get("OriginalFilename")
	if v := get("FileVersion"); v != "" {
		info.FileVersion = v
	}
	if v := get("ProductVersion"); v != "" {
		info.ProductVersion = v
	}
	return info, nil
}

// fixedVersionString renders a packed MS/LS version pair as "a.b.c.d".
func fixedVersionString(ms, ls uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", ms>>16, ms&0xffff, ls>>16, ls&0xffff)
}

// verTranslation mirrors a \VarFileInfo\Translation entry.
type verTranslation struct {
	Lang     uint16
	Codepage uint16
}

// firstTranslation returns the first language/codepage pair of the version
// resource's translation table.
func firstTranslation(block []byte) (lang, cp uint16, ok bool) {
	var translations *verTranslation
	var length uint32
	if err := windows.VerQueryValue(
		unsafe.Pointer(&block[0]),
		`\VarFileInfo\Translation`,
		unsafe.Pointer(&translations),
		&length,
	); err != nil || length < uint32(unsafe.Sizeof(verTranslation{})) {
		return 0, 0, false
	}
	return translations.Lang, translations.Codepage, true
}

// stringTableValue reads a \StringFileInfo value for the given translation,
// returning "" when the value is absent.
func stringTableValue(block []byte, lang, cp uint16, name string) string {
	subBlock := fmt.Sprintf(`\StringFileInfo\%04x%04x\%s`, lang, cp, name)
	var value *uint16
	var valueLen uint32
	if err := windows.VerQueryValue(
		unsafe.Pointer(&block[0]),
		subBlock,
		unsafe.Pointer(&value),
		&valueLen,
	); err != nil || value == nil || valueLen == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString(value))
}
