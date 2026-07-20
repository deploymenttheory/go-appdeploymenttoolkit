package winadt

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
)

// DeferHistory is the persisted deferral state for an install name; storage
// is registry-compatible with PowerShell PSADT deployments of the same app.
type DeferHistory = deploy.DeferHistory

// GetADTDeferHistory is the Go port of Get-ADTDeferHistory.
func GetADTDeferHistory() (DeferHistory, error) {
	s, err := GetADTSession()
	if err != nil {
		return DeferHistory{}, err
	}
	return s.DeferHistory()
}

// SetADTDeferHistory is the Go port of Set-ADTDeferHistory.
func SetADTDeferHistory(h DeferHistory) error {
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	return s.SetDeferHistory(h)
}

// ResetADTDeferHistory is the Go port of Reset-ADTDeferHistory.
func ResetADTDeferHistory() error {
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	return s.ResetDeferHistory()
}
