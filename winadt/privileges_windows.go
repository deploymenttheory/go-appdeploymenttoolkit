package winadt

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// enablePrivileges grants the named privileges to the current process token
// (e.g. SeShutdownPrivilege before InitiateSystemShutdownEx, or
// SeBackupPrivilege+SeRestorePrivilege before RegLoadKey).
func enablePrivileges(names ...string) error {
	var token windows.Token
	if err := windows.OpenProcessToken(
		windows.CurrentProcess(),
		windows.TOKEN_ADJUST_PRIVILEGES|windows.TOKEN_QUERY,
		&token,
	); err != nil {
		return fmt.Errorf("adt: OpenProcessToken: %w", err)
	}
	defer func() { _ = token.Close() }()

	for _, name := range names {
		namePtr, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return fmt.Errorf("adt: encoding privilege name %s: %w", name, err)
		}
		var luid windows.LUID
		if err := windows.LookupPrivilegeValue(nil, namePtr, &luid); err != nil {
			return fmt.Errorf("adt: LookupPrivilegeValue %s: %w", name, err)
		}
		tp := windows.Tokenprivileges{PrivilegeCount: 1}
		tp.Privileges[0] = windows.LUIDAndAttributes{Luid: luid, Attributes: windows.SE_PRIVILEGE_ENABLED}
		if err := windows.AdjustTokenPrivileges(token, false, &tp, 0, nil, nil); err != nil {
			return fmt.Errorf("adt: AdjustTokenPrivileges %s: %w", name, err)
		}
	}
	return nil
}
