package adt

import (
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	sysreg "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/registry"
)

// nativeHiveLoader loads and unloads NTUSER.DAT hives under HKEY_USERS via
// RegLoadKey/RegUnLoadKey. Both calls require SeBackupPrivilege and
// SeRestorePrivilege on the process token.
type nativeHiveLoader struct{}

// Load mounts hiveFile under HKU\<mountKey>.
func (nativeHiveLoader) Load(mountKey, hiveFile string) error {
	if err := enablePrivileges("SeBackupPrivilege", "SeRestorePrivilege"); err != nil {
		return err
	}
	if code := sysreg.RegLoadKey(sysreg.HKEY_USERS, mountKey, hiveFile); code != foundation.ERROR_SUCCESS {
		return fmt.Errorf("adt: RegLoadKey %s (%s): %w",
			mountKey, hiveFile, winerr.FromWin32("RegLoadKey", uint32(code)))
	}
	return nil
}

// Unload unmounts HKU\<mountKey>, flushing the hive back to its file.
func (nativeHiveLoader) Unload(mountKey string) error {
	if code := sysreg.RegUnLoadKey(sysreg.HKEY_USERS, mountKey); code != foundation.ERROR_SUCCESS {
		return fmt.Errorf("adt: RegUnLoadKey %s: %w",
			mountKey, winerr.FromWin32("RegUnLoadKey", uint32(code)))
	}
	return nil
}

// defaultHiveLoader returns the live loader.
func defaultHiveLoader() hiveLoader { return nativeHiveLoader{} }
