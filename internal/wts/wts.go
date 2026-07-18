// Package wts abstracts Windows Terminal Services session queries: who is
// logged on, which session is active on the console, and whether the current
// process can reach an interactive user session. The Windows implementation
// (query_windows.go) uses go-bindings-win32's system/remotedesktop namespace.
package wts

// SessionInfo describes one logged-on user session.
type SessionInfo struct {
	SessionID    uint32
	UserName     string
	DomainName   string
	IsActive     bool // WTSActive connect state
	IsConsole    bool
	IsCurrent    bool // the session this process is running in
	IsRDP        bool
	IsLocalAdmin bool
}

// NTAccount returns DOMAIN\User for the session.
func (s SessionInfo) NTAccount() string {
	if s.DomainName == "" {
		return s.UserName
	}
	return s.DomainName + `\` + s.UserName
}

// Query is the session-enumeration seam.
type Query interface {
	// LoggedOnUsers enumerates interactive user sessions (console and RDP),
	// active first. Empty when nobody is logged on.
	LoggedOnUsers() ([]SessionInfo, error)
	// ProcessSessionID returns the session the current process runs in.
	ProcessSessionID() (uint32, error)
}

// Static is a canned Query for tests and non-Windows builds.
type Static struct {
	Sessions  []SessionInfo
	SessionID uint32
	Err       error
}

// LoggedOnUsers implements Query.
func (s *Static) LoggedOnUsers() ([]SessionInfo, error) {
	return s.Sessions, s.Err
}

// ProcessSessionID implements Query.
func (s *Static) ProcessSessionID() (uint32, error) {
	return s.SessionID, s.Err
}
