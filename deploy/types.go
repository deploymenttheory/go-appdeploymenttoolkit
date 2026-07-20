package deploy

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/deferral"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/session"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/strtab"
)

// DeploymentType is the deployment verb: Install, Uninstall or Repair.
type DeploymentType = session.DeploymentType

// DeploymentType values.
const (
	DeploymentTypeInstall   = session.DeploymentTypeInstall
	DeploymentTypeUninstall = session.DeploymentTypeUninstall
	DeploymentTypeRepair    = session.DeploymentTypeRepair
)

// DeployMode is the interactivity mode: Auto, Interactive, NonInteractive
// or Silent.
type DeployMode = session.DeployMode

// DeployMode values.
const (
	DeployModeAuto           = session.DeployModeAuto
	DeployModeInteractive    = session.DeployModeInteractive
	DeployModeNonInteractive = session.DeployModeNonInteractive
	DeployModeSilent         = session.DeployModeSilent
)

// ParseDeploymentType parses the string form (case-insensitive).
func ParseDeploymentType(s string) (DeploymentType, bool) {
	return session.ParseDeploymentType(s)
}

// ParseDeployMode parses the string form (case-insensitive).
func ParseDeployMode(s string) (DeployMode, bool) {
	return session.ParseDeployMode(s)
}

// ProcessObject describes one process a deployment may need closed (the
// AppProcessesToClose entries).
type ProcessObject = session.ProcessObject

// Config is the resolved toolkit configuration.
type Config = config.Config

// StringTable is the resolved localized string table.
type StringTable = strtab.Table

// EnvironmentTable is the session environment table.
type EnvironmentTable = session.EnvironmentTable

// DeferHistory is the persisted deferral state for an install name.
type DeferHistory = deferral.History
