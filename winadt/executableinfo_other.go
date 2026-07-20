//go:build !windows

package winadt

import "fmt"

// executableVersionInfo is Windows-only (PE version resources); off-Windows it
// surfaces the not-Windows sentinel.
func executableVersionInfo(path string) (ExecutableInfo, error) {
	return ExecutableInfo{}, fmt.Errorf("adt: GetADTExecutableInfo [%s]: %w", path, errNotWindows)
}
