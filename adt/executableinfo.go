package adt

import (
	"context"
	"debug/pe"
	"fmt"
	"os"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// imageSubsystemWindowsGUI is IMAGE_SUBSYSTEM_WINDOWS_GUI: a PE that runs in
// the Windows graphical subsystem rather than the console.
const imageSubsystemWindowsGUI = 2

// ExecutableInfo mirrors PSADT's PSADT.FileSystem.ExecutableInfo: the version
// resource strings of a Windows PE executable plus whether it targets the
// graphical Windows subsystem.
type ExecutableInfo struct {
	ProductName      string
	FileDescription  string
	FileVersion      string
	ProductVersion   string
	CompanyName      string
	InternalName     string
	OriginalFilename string
	IsWindowsApp     bool
}

// GetADTExecutableInfo is the Go port of Get-ADTExecutableInfo: it returns the
// version-resource information for a Windows PE executable, including whether it
// is a graphical (windowed) application.
func GetADTExecutableInfo(ctx context.Context, path string) (ExecutableInfo, error) {
	if err := ctx.Err(); err != nil {
		return ExecutableInfo{}, fmt.Errorf("adt: GetADTExecutableInfo: %w", err)
	}
	if info, err := os.Stat(path); err != nil || info.IsDir() {
		return ExecutableInfo{}, fmt.Errorf("adt: file [%s] does not exist: %w", path, ErrNotFound)
	}
	logToSession(fmt.Sprintf("Retrieving executable info for [%s].", path),
		LogSeverityInfo, "GetADTExecutableInfo")

	info, err := executableVersionInfo(path)
	if err != nil {
		return ExecutableInfo{}, err
	}
	if sub, err := peImageSubsystem(path); err == nil {
		info.IsWindowsApp = isWindowsSubsystem(sub)
	}
	return info, nil
}

// peImageSubsystem reads the PE optional-header Subsystem field. It parses the
// PE format directly (via debug/pe), so it works on any platform.
func peImageSubsystem(path string) (uint16, error) {
	f, err := pe.Open(path)
	if err != nil {
		return 0, fmt.Errorf("adt: parsing PE file [%s]: %w", path, err)
	}
	defer f.Close() //nolint:errcheck // read-only handle

	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader32:
		return oh.Subsystem, nil
	case *pe.OptionalHeader64:
		return oh.Subsystem, nil
	default:
		return 0, winerr.Wrap("adt: file has no PE optional header", winerr.ErrInvalidOption)
	}
}

// isWindowsSubsystem reports whether a PE subsystem value denotes a graphical
// (windowed) Windows application.
func isWindowsSubsystem(subsystem uint16) bool {
	return subsystem == imageSubsystemWindowsGUI
}
