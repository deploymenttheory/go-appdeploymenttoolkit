package session

import (
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/globalization"
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
