package psadt

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/strtab"
)

// Config is the resolved toolkit configuration (config.psd1 parity).
type Config = config.Config

// StringTable is the resolved localized string table.
type StringTable = strtab.Table

// EnvironmentTable is the session environment table.
type EnvironmentTable = session.EnvironmentTable

// GetADTConfig is the Go port of Get-ADTConfig: it returns the active
// session's resolved configuration.
func GetADTConfig() (*Config, error) {
	s, err := GetADTSession()
	if err != nil {
		return nil, err
	}
	return s.Config(), nil
}

// GetADTStringTable is the Go port of Get-ADTStringTable: it returns the
// active session's resolved string table.
func GetADTStringTable() (*StringTable, error) {
	s, err := GetADTSession()
	if err != nil {
		return nil, err
	}
	return s.Strings(), nil
}

// GetADTEnvironmentTable is the Go port of Get-ADTEnvironmentTable: it
// returns the active session's environment table.
func GetADTEnvironmentTable() (*EnvironmentTable, error) {
	s, err := GetADTSession()
	if err != nil {
		return nil, err
	}
	return s.Environment(), nil
}
