//go:build !windows

package psadt

import "fmt"

// executableVersionInfo is Windows-only (PE version resources); off-Windows it
// surfaces the not-Windows sentinel.
func executableVersionInfo(path string) (ExecutableInfo, error) {
	return ExecutableInfo{}, fmt.Errorf("psadt: GetADTExecutableInfo [%s]: %w", path, errNotWindows)
}
