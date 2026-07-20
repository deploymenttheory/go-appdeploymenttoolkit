package winadt

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
)

// Config is the resolved toolkit configuration (config.psd1 parity).
type Config = deploy.Config

// StringTable is the resolved localized string table.
type StringTable = deploy.StringTable

// EnvironmentTable is the session environment table.
type EnvironmentTable = deploy.EnvironmentTable

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
