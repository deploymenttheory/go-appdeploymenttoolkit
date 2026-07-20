//go:build !windows

package adt

import "github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"

// stubHiveLoader reports ErrNotWindows; hive loading needs the Windows
// registry. The all-users orchestration is still testable off-Windows via
// the fake backend as long as every hive is already loaded.
type stubHiveLoader struct{}

func (stubHiveLoader) Load(_, _ string) error {
	return winerr.Wrap("adt: loading registry hives requires Windows", winerr.ErrNotWindows)
}

func (stubHiveLoader) Unload(_ string) error {
	return winerr.Wrap("adt: unloading registry hives requires Windows", winerr.ErrNotWindows)
}

// defaultHiveLoader returns the stub loader.
func defaultHiveLoader() hiveLoader { return stubHiveLoader{} }
