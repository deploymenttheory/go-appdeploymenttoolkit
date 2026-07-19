//go:build !windows

package msipkg

import "github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"

// TableProperty requires the Windows Installer runtime.
func TableProperty(msiPath, property string) (string, error) {
	return "", winerr.Wrap("msipkg: TableProperty "+msiPath, winerr.ErrNotWindows)
}

// AllProperties requires the Windows Installer runtime.
func AllProperties(msiPath string) (map[string]string, error) {
	return nil, winerr.Wrap("msipkg: AllProperties "+msiPath, winerr.ErrNotWindows)
}

// SetProperty requires the Windows Installer runtime.
func SetProperty(msiPath, property, value string) error {
	return winerr.Wrap("msipkg: SetProperty "+msiPath, winerr.ErrNotWindows)
}

// CreatePropertyTransform requires the Windows Installer runtime.
func CreatePropertyTransform(
	msiPath, newTransformPath, applyTransformPath string,
	properties map[string]string,
) error {
	return winerr.Wrap("msipkg: CreatePropertyTransform "+msiPath, winerr.ErrNotWindows)
}
