package wts

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"

	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/remotedesktop"
)

// Native queries the live WTS subsystem.
type Native struct{}

// NewNative returns the Windows WTS query implementation.
func NewNative() *Native { return &Native{} }

// LoggedOnUsers implements Query via WTSEnumerateSessions +
// WTSQuerySessionInformation, mirroring PSADT's TerminalServerUtilities.
func (n *Native) LoggedOnUsers() ([]SessionInfo, error) {
	var info *remotedesktop.WTS_SESSION_INFOW
	var count uint32
	if err := remotedesktop.WTSEnumerateSessions(0, 0, 1, &info, &count); err != nil {
		return nil, fmt.Errorf("wts: enumerating sessions: %w", err)
	}
	defer remotedesktop.WTSFreeMemory(unsafe.Pointer(info))

	currentID, err := n.ProcessSessionID()
	if err != nil {
		currentID = ^uint32(0)
	}
	consoleID := remotedesktop.WTSGetActiveConsoleSessionId()

	sessions := unsafe.Slice(info, count)
	out := make([]SessionInfo, 0, count)
	for i := range sessions {
		s := &sessions[i]
		user, err := querySessionString(s.SessionId, remotedesktop.WTSUserName)
		if err != nil || user == "" {
			continue // service/listen sessions have no user
		}
		domain, _ := querySessionString(s.SessionId, remotedesktop.WTSDomainName)
		station := win32.UTF16ToString(s.PWinStationName)
		out = append(out, SessionInfo{
			SessionID:  s.SessionId,
			UserName:   user,
			DomainName: domain,
			IsActive:   s.State == remotedesktop.WTSActive,
			IsConsole:  s.SessionId == consoleID,
			IsCurrent:  s.SessionId == currentID,
			IsRDP:      len(station) >= 3 && station[:3] == "RDP",
		})
	}
	// Active sessions first.
	for i, j := 0, 0; j < len(out); j++ {
		if out[j].IsActive {
			out[i], out[j] = out[j], out[i]
			i++
		}
	}
	return out, nil
}

func querySessionString(sessionID uint32, class remotedesktop.WTS_INFO_CLASS) (string, error) {
	var buf foundation.PWSTR
	var bytes uint32
	if err := remotedesktop.WTSQuerySessionInformation(
		0,
		sessionID,
		class,
		&buf,
		&bytes,
	); err != nil {
		return "", fmt.Errorf("wts: querying session %d info %d: %w", sessionID, class, err)
	}
	defer remotedesktop.WTSFreeMemory(unsafe.Pointer(buf))
	return win32.UTF16ToString(buf), nil
}

// ProcessSessionID implements Query.
func (n *Native) ProcessSessionID() (uint32, error) {
	var id uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &id); err != nil {
		return 0, fmt.Errorf("wts: resolving process session: %w", err)
	}
	return id, nil
}
