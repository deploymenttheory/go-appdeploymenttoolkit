package psadt

import (
	"context"
	"fmt"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
)

// LoggedOnUser describes an interactive user session, mirroring the core of
// Get-ADTLoggedOnUser's output object.
type LoggedOnUser = wts.SessionInfo

// UserProfile mirrors the core of Get-ADTUserProfiles' output: one local
// user profile from the ProfileList registry.
type UserProfile struct {
	NTAccount   string
	SID         string
	ProfilePath string
}

// GetADTUserProfilesOptions mirrors Get-ADTUserProfiles' parameters.
type GetADTUserProfilesOptions struct {
	// ExcludeNTAccount lists DOMAIN\User accounts to omit.
	ExcludeNTAccount []string
	// IncludeSystemProfiles includes SYSTEM/LOCAL SERVICE/NETWORK SERVICE.
	IncludeSystemProfiles bool
	// IncludeServiceProfiles includes NT SERVICE\* profiles.
	IncludeServiceProfiles bool
	// ExcludeDefaultUser omits the Default template profile.
	ExcludeDefaultUser bool
}

// GetADTLoggedOnUser is the Go port of Get-ADTLoggedOnUser: it returns the
// interactive user sessions (console and RDP), active sessions first.
func GetADTLoggedOnUser(ctx context.Context) ([]LoggedOnUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("psadt: %w", err)
	}
	return wtsQuery().LoggedOnUsers()
}

// GetADTUserProfiles is the Go port of Get-ADTUserProfiles: it enumerates
// local user profiles from the ProfileList registry key, excluding system
// and service profiles by default.
func GetADTUserProfiles(ctx context.Context, opts GetADTUserProfilesOptions) ([]UserProfile, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("psadt: %w", err)
	}
	return userProfiles(ctx, opts)
}

// ConvertToADTNTAccountOrSID is the Go port of ConvertTo-ADTNTAccountOrSID:
// it converts between NT account names (DOMAIN\User) and SID strings,
// detecting the input form automatically.
func ConvertToADTNTAccountOrSID(ctx context.Context, accountOrSID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("psadt: %w", err)
	}
	return convertAccountOrSID(accountOrSID)
}

// OperatingSystemInfo mirrors the core of Get-ADTOperatingSystemInfo.
type OperatingSystemInfo struct {
	Name         string // e.g. "Windows 11 Enterprise"
	Version      string // e.g. "10.0.26100"
	Build        string
	DisplayVersion string // e.g. "24H2"
	Architecture string
	IsServer     bool
	Is64Bit      bool
}

// GetADTOperatingSystemInfo is the Go port of Get-ADTOperatingSystemInfo.
func GetADTOperatingSystemInfo(ctx context.Context) (*OperatingSystemInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("psadt: %w", err)
	}
	return operatingSystemInfo()
}
