// Package session implements the deployment session engine: the Go port of
// PSADT's DeploymentSession (Foundation/DeploymentSession.cs) driving
// configuration, logging, deferral and exit-code semantics.
package session

import "strings"

// DeploymentType mirrors PSADT's Install/Uninstall/Repair verbs.
type DeploymentType int

// DeploymentType values.
const (
	DeploymentTypeInstall DeploymentType = iota
	DeploymentTypeUninstall
	DeploymentTypeRepair
)

func (t DeploymentType) String() string {
	switch t {
	case DeploymentTypeUninstall:
		return "Uninstall"
	case DeploymentTypeRepair:
		return "Repair"
	default:
		return "Install"
	}
}

// Verb returns the present-tense noun used in log strings
// ("installation", "uninstallation", "repair").
func (t DeploymentType) Verb() string {
	switch t {
	case DeploymentTypeUninstall:
		return "uninstallation"
	case DeploymentTypeRepair:
		return "repair"
	default:
		return "installation"
	}
}

// ParseDeploymentType parses the PSADT string form (case-insensitive).
func ParseDeploymentType(s string) (DeploymentType, bool) {
	switch strings.ToLower(s) {
	case "", "install":
		return DeploymentTypeInstall, true
	case "uninstall":
		return DeploymentTypeUninstall, true
	case "repair":
		return DeploymentTypeRepair, true
	default:
		return DeploymentTypeInstall, false
	}
}

// DeployMode mirrors PSADT's Auto/Interactive/NonInteractive/Silent modes.
type DeployMode int

// DeployMode values.
const (
	DeployModeAuto DeployMode = iota
	DeployModeInteractive
	DeployModeNonInteractive
	DeployModeSilent
)

func (m DeployMode) String() string {
	switch m {
	case DeployModeInteractive:
		return "Interactive"
	case DeployModeNonInteractive:
		return "NonInteractive"
	case DeployModeSilent:
		return "Silent"
	default:
		return "Auto"
	}
}

// ParseDeployMode parses the PSADT string form (case-insensitive).
func ParseDeployMode(s string) (DeployMode, bool) {
	switch strings.ToLower(s) {
	case "", "auto":
		return DeployModeAuto, true
	case "interactive":
		return DeployModeInteractive, true
	case "noninteractive":
		return DeployModeNonInteractive, true
	case "silent":
		return DeployModeSilent, true
	default:
		return DeployModeAuto, false
	}
}

// Status mirrors PSADT's DeploymentStatus classification of an exit code.
type Status int

// Status values.
const (
	StatusComplete Status = iota
	StatusRestartRequired
	StatusFastRetry
	StatusError
)

func (s Status) String() string {
	switch s {
	case StatusRestartRequired:
		return "RestartRequired"
	case StatusFastRetry:
		return "FastRetry"
	case StatusError:
		return "Error"
	default:
		return "Complete"
	}
}

// ProcessObject mirrors PSADT's process-to-close descriptor.
type ProcessObject struct {
	Name        string // process name without extension, e.g. "excel"
	Description string // friendly name shown in the close-apps dialog
}
