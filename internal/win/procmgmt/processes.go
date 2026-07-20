package procmgmt

import "strings"

// ProcessSpec names a process to look for, the procmgmt analogue of PSADT's
// ProcessDefinition (session.ProcessObject at the facade). Name is matched
// case-insensitively with or without the ".exe" extension.
type ProcessSpec struct {
	Name        string // process name, e.g. "excel" or "excel.exe"
	Description string // optional friendly name override
}

// RunningProcess is one matched running process, the Go analogue of PSADT's
// RunningProcessInfo.
type RunningProcess struct {
	Name        string // process name without the ".exe" extension
	Description string // spec override, else PE version FileDescription, else Name
	PID         uint32
	SessionID   uint32
}

// WindowInfo describes one visible top-level window, the Go analogue of
// PSADT's WindowInfo.
type WindowInfo struct {
	Handle uintptr
	Title  string
	PID    uint32
}

// ProcessesBySessionID filters matches down to one WTS session.
func ProcessesBySessionID(procs []RunningProcess, sessionID uint32) []RunningProcess {
	out := make([]RunningProcess, 0, len(procs))
	for _, p := range procs {
		if p.SessionID == sessionID {
			out = append(out, p)
		}
	}
	return out
}

// normalizeProcessName lowercases a process name and strips a trailing
// ".exe" so specs match Toolhelp entries regardless of extension or case.
func normalizeProcessName(name string) string {
	name = strings.ToLower(name)
	return strings.TrimSuffix(name, ".exe")
}

// trimExeSuffix removes a trailing ".exe" (any case) preserving the rest of
// the original casing for display.
func trimExeSuffix(name string) string {
	if strings.HasSuffix(strings.ToLower(name), ".exe") {
		return name[:len(name)-len(".exe")]
	}
	return name
}
