package procmgmt

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/wts"
)

// DenyTerminateToUser prepends a deny-PROCESS_TERMINATE ACE for the given
// SID to the process's kernel-object DACL, porting Start-ADTProcess's
// -DenyUserTermination. The rest of the DACL is preserved.
func DenyTerminateToUser(pid int, sid *windows.SID) error {
	//#nosec G115 -- pid is a Windows process ID
	proc, err := windows.OpenProcess(
		windows.READ_CONTROL|windows.WRITE_DAC,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("procmgmt: OpenProcess(%d) for DACL edit: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()

	sd, err := windows.GetSecurityInfo(proc, windows.SE_KERNEL_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("procmgmt: GetSecurityInfo: %w", err)
	}
	oldDacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("procmgmt: reading process DACL: %w", err)
	}
	entry := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.PROCESS_TERMINATE,
		AccessMode:        windows.DENY_ACCESS,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_USER,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
	newDacl, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry}, oldDacl)
	if err != nil {
		return fmt.Errorf("procmgmt: building deny-terminate DACL: %w", err)
	}
	if err := windows.SetSecurityInfo(
		proc,
		windows.SE_KERNEL_OBJECT,
		windows.DACL_SECURITY_INFORMATION,
		nil, nil, newDacl, nil,
	); err != nil {
		return fmt.Errorf("procmgmt: SetSecurityInfo: %w", err)
	}
	return nil
}

// InteractiveUserSID returns the SID of the first active interactive user
// session, or nil when none is resolvable. Used as the DenyUserTermination
// trustee for own-session launches.
func InteractiveUserSID() *windows.SID {
	users, err := wts.NewNative().LoggedOnUsers()
	if err != nil {
		return nil
	}
	for _, u := range users {
		if !u.IsActive || u.UserName == "" {
			continue
		}
		sid, _, _, err := windows.LookupSID("", u.NTAccount())
		if err != nil {
			continue
		}
		return sid
	}
	return nil
}
