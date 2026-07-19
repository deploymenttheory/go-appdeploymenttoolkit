package adt

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
)

func wtsQuery() wts.Query { return wts.NewNative() }

const profileListKey = `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`

// userProfiles enumerates the ProfileList registry key.
func userProfiles(_ context.Context, opts GetADTUserProfilesOptions) ([]UserProfile, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, profileListKey, registry.READ)
	if err != nil {
		return nil, fmt.Errorf("adt: opening ProfileList: %w", err)
	}
	defer k.Close()
	sids, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("adt: enumerating ProfileList: %w", err)
	}

	exclude := make(map[string]bool, len(opts.ExcludeNTAccount))
	for _, e := range opts.ExcludeNTAccount {
		exclude[strings.ToLower(e)] = true
	}

	profiles := make([]UserProfile, 0, len(sids))
	for _, sid := range sids {
		if isSystemProfileSID(sid) && !opts.IncludeSystemProfiles {
			continue
		}
		if strings.HasPrefix(sid, "S-1-5-80-") && !opts.IncludeServiceProfiles {
			continue // NT SERVICE virtual accounts
		}
		sub, err := registry.OpenKey(k, sid, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		path, _, err := sub.GetStringValue("ProfileImagePath")
		_ = sub.Close()
		if err != nil || path == "" {
			continue
		}
		account, err := convertAccountOrSID(sid)
		if err != nil {
			account = "" // orphaned SID; keep the profile with its path
		}
		if account != "" && exclude[strings.ToLower(account)] {
			continue
		}
		profiles = append(profiles, UserProfile{
			NTAccount:   account,
			SID:         sid,
			ProfilePath: expandRegistryString(path),
		})
	}

	if !opts.ExcludeDefaultUser {
		if def, _, err := k.GetStringValue("Default"); err == nil && def != "" {
			profiles = append(profiles, UserProfile{
				NTAccount:   "Default User",
				ProfilePath: expandRegistryString(def),
			})
		}
	}
	return profiles, nil
}

// isSystemProfileSID matches SYSTEM (S-1-5-18), LOCAL SERVICE (19) and
// NETWORK SERVICE (20).
func isSystemProfileSID(sid string) bool {
	switch sid {
	case "S-1-5-18", "S-1-5-19", "S-1-5-20":
		return true
	default:
		return false
	}
}

func expandRegistryString(s string) string {
	out, err := registry.ExpandString(s)
	if err != nil {
		return s
	}
	return out
}

// convertAccountOrSID converts SID→NTAccount or NTAccount→SID depending on
// the input's shape.
func convertAccountOrSID(accountOrSID string) (string, error) {
	if strings.HasPrefix(strings.ToUpper(accountOrSID), "S-1-") {
		sid, err := windows.StringToSid(accountOrSID)
		if err != nil {
			return "", fmt.Errorf("adt: parsing SID %s: %w", accountOrSID, err)
		}
		user, domain, _, err := sid.LookupAccount("")
		if err != nil {
			return "", fmt.Errorf("adt: resolving SID %s: %w", accountOrSID, err)
		}
		if domain == "" {
			return user, nil
		}
		return domain + `\` + user, nil
	}
	sid, _, _, err := windows.LookupSID("", accountOrSID)
	if err != nil {
		return "", fmt.Errorf("adt: resolving account %s: %w", accountOrSID, err)
	}
	out, err := sid.String(), error(nil)
	if out == "" {
		return "", winerr.Wrap("adt: SID for "+accountOrSID, winerr.ErrNotFound)
	}
	return out, err
}

// operatingSystemInfo reads version facts from the kernel and registry.
func operatingSystemInfo() (*OperatingSystemInfo, error) {
	major, minor, build := windows.RtlGetNtVersionNumbers()

	info := &OperatingSystemInfo{
		Version: fmt.Sprintf("%d.%d.%d", major, minor, build),
		Build:   fmt.Sprintf("%d", build),
		Is64Bit: true,
	}
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion`, registry.QUERY_VALUE)
	if err != nil {
		return info, nil //nolint:nilerr // version numbers alone are still useful
	}
	defer k.Close()
	if v, _, err := k.GetStringValue("ProductName"); err == nil {
		info.Name = v
	}
	if v, _, err := k.GetStringValue("DisplayVersion"); err == nil {
		info.DisplayVersion = v
	}
	if v, _, err := k.GetStringValue("InstallationType"); err == nil {
		info.IsServer = strings.EqualFold(v, "Server")
	}
	switch runtime.GOARCH {
	case "amd64":
		info.Architecture = "x64"
	case "arm64":
		info.Architecture = "ARM64"
	default:
		info.Architecture = "x86"
		info.Is64Bit = false
	}
	return info, nil
}
