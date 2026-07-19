//go:build !windows

package adt

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
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
