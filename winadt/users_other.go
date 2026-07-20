//go:build !windows

package winadt

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/wts"
)

func wtsQuery() wts.Query { return &wts.Static{} }

func userProfiles(context.Context, GetADTUserProfilesOptions) ([]UserProfile, error) {
	return nil, winerr.ErrNotWindows
}

func convertAccountOrSID(string) (string, error) {
	return "", winerr.ErrNotWindows
}

func operatingSystemInfo() (*OperatingSystemInfo, error) {
	return nil, winerr.ErrNotWindows
}
