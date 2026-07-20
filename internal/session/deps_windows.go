package session

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/globalization"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/setupandmigration"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/stationsanddesktops"
)

func defaultRegistry() regkey.Backend { return regkey.NewNative() }

func defaultWTS() wts.Query { return wts.NewNative() }

// defaultIsAdmin reports membership of the Administrators group for the
// current process token (parity with Test-ADTCallerIsAdmin).
func defaultIsAdmin() bool {
	sid, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return false
	}
	token := windows.GetCurrentProcessToken()
	member, err := token.IsMember(sid)
	if err != nil {
		return false
	}
	return member || token.IsElevated()
}

// defaultOobeCompleted calls kernel32!OOBEComplete.
func defaultOobeCompleted() (bool, error) {
	var complete foundation.BOOL
	if err := setupandmigration.OOBEComplete(&complete); err != nil {
		return false, fmt.Errorf("session: OOBEComplete: %w", err)
	}
	return complete != 0, nil
}

// defaultProcessRunning reports whether any of the named processes is running.
func defaultProcessRunning(names []string) (bool, error) {
	if len(names) == 0 {
		return false, nil
	}
	specs := make([]procmgmt.ProcessSpec, len(names))
	for i, n := range names {
		specs[i] = procmgmt.ProcessSpec{Name: n}
	}
	procs, err := procmgmt.RunningProcesses(specs)
	if err != nil {
		return false, err
	}
	return len(procs) > 0, nil
}

// defaultProcessInteractive reports whether the process window station is the
// interactive one (WinSta0), the Environment.UserInteractive equivalent.
func defaultProcessInteractive() bool {
	hStation, err := stationsanddesktops.GetProcessWindowStation()
	if err != nil {
		return false
	}
	buf := make([]byte, 256)
	var needed uint32
	if err := stationsanddesktops.GetUserObjectInformation(
		foundation.HANDLE(hStation),
		stationsanddesktops.UOI_NAME,
		buf,
		&needed,
	); err != nil {
		return false
	}
	name := win32.UTF16ToString((*uint16)(unsafe.Pointer(unsafe.SliceData(buf))))
	return strings.EqualFold(name, "WinSta0")
}

// defaultActiveUserSID returns the SID of the first active interactive user
// session, or "" when nobody is active or lookup fails.
func defaultActiveUserSID() string {
	users, err := defaultWTS().LoggedOnUsers()
	if err != nil {
		return ""
	}
	for _, u := range users {
		if !u.IsActive || u.UserName == "" {
			continue
		}
		sid, _, _, err := windows.LookupSID("", u.NTAccount())
		if err != nil {
			continue
		}
		return sid.String()
	}
	return ""
}

// defaultCulture returns the user's default locale name (e.g. "en-US").
func defaultCulture() string {
	const localeNameMaxLength = int32(85) // LOCALE_NAME_MAX_LENGTH
	buf := make([]uint16, localeNameMaxLength)
	n, err := globalization.GetUserDefaultLocaleName(
		foundation.PWSTR(unsafe.SliceData(buf)),
		localeNameMaxLength,
	)
	if err != nil || n <= 0 {
		return ""
	}
	return win32.UTF16ToString(unsafe.SliceData(buf))
}
