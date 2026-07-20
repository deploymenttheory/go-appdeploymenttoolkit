package fsops

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SetItemPermission is the engine behind Set-ADTItemPermission: it applies
// grant/deny/remove ACL changes and inheritance toggles to a file, folder or
// registry key via the named-security-info APIs.
func SetItemPermission(ctx context.Context, opts ItemPermissionOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("fsops: %w", err)
	}
	if err := opts.Validate(); err != nil {
		return err
	}
	objName, objType := securityObject(opts.Path)

	dacl, err := currentDACL(objName, objType)
	if err != nil {
		return err
	}

	if opts.EnableInheritance {
		return setDACL(objName, objType,
			windows.DACL_SECURITY_INFORMATION|windows.UNPROTECTED_DACL_SECURITY_INFORMATION, dacl)
	}

	if opts.DisableInheritance {
		protected := dacl
		if !opts.PreserveAccessRules {
			if protected, err = explicitOnlyACL(dacl); err != nil {
				return err
			}
		}
		secInfo := windows.SECURITY_INFORMATION(
			windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION,
		)
		if err := setDACL(objName, objType, secInfo, protected); err != nil {
			return err
		}
		if strings.TrimSpace(opts.User) == "" {
			return nil
		}
		// Re-read so the permission change below merges with the
		// now-protected ACL.
		if dacl, err = currentDACL(objName, objType); err != nil {
			return err
		}
	}

	sid, err := resolveSID(opts.User)
	if err != nil {
		return err
	}
	var pinner runtime.Pinner
	pinner.Pin(sid)
	defer pinner.Unpin()

	mask, _ := opts.Permission.AccessMask() // validated above; Remove ignores it
	entry := windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.ACCESS_MASK(mask),
		AccessMode:        accessModeFor(opts.Action),
		Inheritance:       inheritanceFlags(opts.Inheritance),
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
	newACL, err := windows.ACLFromEntries([]windows.EXPLICIT_ACCESS{entry}, dacl)
	if err != nil {
		return fmt.Errorf("fsops: SetEntriesInAcl for %s: %w", opts.Path, err)
	}
	return setDACL(objName, objType, windows.DACL_SECURITY_INFORMATION, newACL)
}

// currentDACL reads the object's discretionary ACL.
func currentDACL(objName string, objType windows.SE_OBJECT_TYPE) (*windows.ACL, error) {
	sd, err := windows.GetNamedSecurityInfo(objName, objType, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return nil, fmt.Errorf("fsops: GetNamedSecurityInfo %s: %w", objName, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return nil, fmt.Errorf("fsops: reading DACL of %s: %w", objName, err)
	}
	return dacl, nil
}

// setDACL writes the DACL back with the given security-information flags.
func setDACL(
	objName string,
	objType windows.SE_OBJECT_TYPE,
	secInfo windows.SECURITY_INFORMATION,
	dacl *windows.ACL,
) error {
	if err := windows.SetNamedSecurityInfo(
		objName,
		objType,
		secInfo,
		nil,
		nil,
		dacl,
		nil,
	); err != nil {
		return fmt.Errorf("fsops: SetNamedSecurityInfo %s: %w", objName, err)
	}
	return nil
}

// explicitOnlyACL rebuilds the ACL keeping only non-inherited entries,
// mirroring SetAccessRuleProtection(true, false).
func explicitOnlyACL(acl *windows.ACL) (*windows.ACL, error) {
	var entries []windows.EXPLICIT_ACCESS
	var pinner runtime.Pinner
	defer pinner.Unpin()
	if acl != nil {
		for i := uint32(0); i < uint32(acl.AceCount); i++ {
			var ace *windows.ACCESS_ALLOWED_ACE
			if err := windows.GetAce(acl, i, &ace); err != nil {
				return nil, fmt.Errorf("fsops: GetAce(%d): %w", i, err)
			}
			if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
				continue // drop entries inherited from the parent
			}
			mode := windows.ACCESS_MODE(windows.GRANT_ACCESS)
			if ace.Header.AceType == windows.ACCESS_DENIED_ACE_TYPE {
				mode = windows.DENY_ACCESS
			}
			// ACCESS_ALLOWED_ACE and ACCESS_DENIED_ACE share this
			// layout; the SID begins at SidStart.
			sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
			pinner.Pin(sid)
			entries = append(entries, windows.EXPLICIT_ACCESS{
				AccessPermissions: ace.Mask,
				AccessMode:        mode,
				Inheritance: uint32(
					ace.Header.AceFlags,
				) & windows.SUB_CONTAINERS_AND_OBJECTS_INHERIT,
				Trustee: windows.TRUSTEE{
					TrusteeForm:  windows.TRUSTEE_IS_SID,
					TrusteeType:  windows.TRUSTEE_IS_UNKNOWN,
					TrusteeValue: windows.TrusteeValueFromSID(sid),
				},
			})
		}
	}
	newACL, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return nil, fmt.Errorf("fsops: rebuilding explicit ACL: %w", err)
	}
	return newACL, nil
}

// accessModeFor maps the port's action to the SetEntriesInAcl access mode.
func accessModeFor(a AccessAction) windows.ACCESS_MODE {
	switch a {
	case ActionDeny:
		return windows.DENY_ACCESS
	case ActionRemove:
		return windows.REVOKE_ACCESS
	default:
		return windows.GRANT_ACCESS
	}
}

// inheritanceFlags maps InheritanceScope onto EXPLICIT_ACCESS inheritance.
func inheritanceFlags(s InheritanceScope) uint32 {
	var flags uint32 = windows.NO_INHERITANCE
	if s&InheritObject != 0 {
		flags |= windows.SUB_OBJECTS_ONLY_INHERIT
	}
	if s&InheritContainer != 0 {
		flags |= windows.SUB_CONTAINERS_ONLY_INHERIT
	}
	return flags
}

// resolveSID resolves an NTAccount name or SID string (optionally in PSADT's
// "*S-1-..." form) to a SID.
func resolveSID(user string) (*windows.SID, error) {
	account := strings.TrimPrefix(strings.TrimSpace(user), "*")
	if strings.HasPrefix(strings.ToUpper(account), "S-1-") {
		sid, err := windows.StringToSid(account)
		if err != nil {
			return nil, fmt.Errorf("fsops: converting SID %s: %w", account, err)
		}
		return sid, nil
	}
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		return nil, fmt.Errorf("fsops: resolving account %s: %w", account, err)
	}
	return sid, nil
}

// registryHives maps path prefixes onto the SE_REGISTRY_KEY root names the
// named-security-info APIs expect.
var registryHives = map[string]string{
	"HKEY_LOCAL_MACHINE":  "MACHINE",
	"HKLM":                "MACHINE",
	"HKEY_CURRENT_USER":   "CURRENT_USER",
	"HKCU":                "CURRENT_USER",
	"HKEY_USERS":          "USERS",
	"HKU":                 "USERS",
	"HKEY_CLASSES_ROOT":   "CLASSES_ROOT",
	"HKCR":                "CLASSES_ROOT",
	"HKEY_CURRENT_CONFIG": "CURRENT_CONFIG",
	"HKCC":                "CURRENT_CONFIG",
}

// securityObject classifies the path as a registry key or file object and
// normalizes registry hive prefixes.
func securityObject(path string) (string, windows.SE_OBJECT_TYPE) {
	hive, rest, _ := strings.Cut(strings.ReplaceAll(path, "/", `\`), `\`)
	if root, ok := registryHives[strings.ToUpper(strings.TrimSuffix(hive, ":"))]; ok {
		if rest == "" {
			return root, windows.SE_REGISTRY_KEY
		}
		return root + `\` + rest, windows.SE_REGISTRY_KEY
	}
	return path, windows.SE_FILE_OBJECT
}
