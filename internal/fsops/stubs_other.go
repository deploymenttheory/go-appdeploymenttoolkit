//go:build !windows

package fsops

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// FileVersionInfo carries a file's version resources: FileVersion is the
// binary VS_FIXEDFILEINFO version ("a.b.c.d"); ProductVersion is the string
// table's ProductVersion when present (it can differ from the fixed value),
// otherwise the fixed product version.
type FileVersionInfo struct {
	FileVersion    string
	ProductVersion string
}

// GetFileVersion is Windows-only (version resources are a PE concept).
func GetFileVersion(_ string) (FileVersionInfo, error) {
	return FileVersionInfo{}, winerr.Wrap("fsops: file version query", winerr.ErrNotWindows)
}

// FreeDiskSpaceMB is Windows-only in this toolkit.
func FreeDiskSpaceMB(_ string) (uint64, error) {
	return 0, winerr.Wrap("fsops: free disk space query", winerr.ErrNotWindows)
}

// SetItemPermission is Windows-only (Windows ACL semantics).
func SetItemPermission(_ context.Context, opts ItemPermissionOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	return winerr.Wrap("fsops: item permissions", winerr.ErrNotWindows)
}
