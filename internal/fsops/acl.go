package fsops

import (
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// AccessAction selects how an access-control entry is applied, mirroring
// Set-ADTItemPermission's Allow/Deny types and its removal methods.
type AccessAction int

// AccessAction values.
const (
	// ActionGrant adds an allow ACE for the user (AddAccessRule + Allow).
	ActionGrant AccessAction = iota
	// ActionDeny adds a deny ACE for the user (AddAccessRule + Deny).
	ActionDeny
	// ActionRemove revokes all explicit access for the user
	// (RemoveAccessRuleAll).
	ActionRemove
)

func (a AccessAction) String() string {
	switch a {
	case ActionDeny:
		return "Deny"
	case ActionRemove:
		return "Remove"
	default:
		return "Grant"
	}
}

// Permission is a PSADT simplified permission-set name.
type Permission string

// Permission values (subset of System.Security.AccessControl.FileSystemRights
// supported by the port).
const (
	PermissionFullControl    Permission = "FullControl"
	PermissionModify         Permission = "Modify"
	PermissionReadAndExecute Permission = "ReadAndExecute"
	PermissionRead           Permission = "Read"
	PermissionWrite          Permission = "Write"
)

// Generic access rights (winnt.h) used to express the simplified permission
// sets; kept portable for validation tests.
const (
	genericRead    = 0x80000000
	genericWrite   = 0x40000000
	genericExecute = 0x20000000
	genericAll     = 0x10000000
	deleteRight    = 0x00010000 // DELETE standard right
)

// AccessMask resolves the simplified permission name to a generic access
// mask, reporting false for unknown names.
func (p Permission) AccessMask() (uint32, bool) {
	switch Permission(strings.TrimSpace(string(p))) {
	case PermissionFullControl:
		return genericAll, true
	case PermissionModify:
		return genericRead | genericWrite | genericExecute | deleteRight, true
	case PermissionReadAndExecute:
		return genericRead | genericExecute, true
	case PermissionRead:
		return genericRead, true
	case PermissionWrite:
		return genericWrite, true
	default:
		return 0, false
	}
}

// InheritanceScope is a bitmask mirroring
// System.Security.AccessControl.InheritanceFlags.
type InheritanceScope int

// InheritanceScope values; combine ObjectInherit|ContainerInherit to apply
// to both files and subfolders.
const (
	InheritNone      InheritanceScope = 0
	InheritObject    InheritanceScope = 1 // OBJECT_INHERIT_ACE
	InheritContainer InheritanceScope = 2 // CONTAINER_INHERIT_ACE
)

// ItemPermissionOptions mirrors the parameters of Set-ADTItemPermission.
type ItemPermissionOptions struct {
	// Path is the file, folder or registry key to modify. Registry paths
	// use hive prefixes (HKLM\..., HKEY_LOCAL_MACHINE\...).
	Path string
	// User is an NTAccount ("DOMAIN\User", "BUILTIN\Users") or a SID
	// string ("S-1-5-18", or "*S-1-5-18" in PSADT's asterisk form).
	User string
	// Action selects Grant/Deny/Remove for User.
	Action AccessAction
	// Permission is the simplified permission set to apply.
	Permission Permission
	// Inheritance controls how the ACE propagates to children.
	Inheritance InheritanceScope
	// EnableInheritance re-enables ACL inheritance from the parent
	// (standalone operation; User/Permission are ignored).
	EnableInheritance bool
	// DisableInheritance protects the object's ACL from parent
	// inheritance before any permission change is applied.
	DisableInheritance bool
	// PreserveAccessRules keeps inherited entries as explicit copies when
	// disabling inheritance (Set-ADTItemPermission always preserves).
	PreserveAccessRules bool
}

// Validate checks option consistency; it is the portable half of
// SetItemPermission.
func (o ItemPermissionOptions) Validate() error {
	if strings.TrimSpace(o.Path) == "" {
		return fmt.Errorf("fsops: item permission requires a path: %w", winerr.ErrInvalidOption)
	}
	if o.EnableInheritance && o.DisableInheritance {
		return fmt.Errorf(
			"fsops: EnableInheritance and DisableInheritance are mutually exclusive: %w",
			winerr.ErrInvalidOption,
		)
	}
	if o.EnableInheritance {
		return nil // standalone inheritance toggle
	}
	hasUser := strings.TrimSpace(o.User) != ""
	if !hasUser {
		if o.DisableInheritance {
			return nil // standalone protection toggle
		}
		return fmt.Errorf(
			"fsops: item permission requires a user or group: %w",
			winerr.ErrInvalidOption,
		)
	}
	if o.Action != ActionRemove {
		if _, ok := o.Permission.AccessMask(); !ok {
			return fmt.Errorf(
				"fsops: unknown permission set %q: %w",
				o.Permission,
				winerr.ErrInvalidOption,
			)
		}
	}
	if o.Inheritance&^(InheritObject|InheritContainer) != 0 {
		return fmt.Errorf(
			"fsops: invalid inheritance scope %d: %w",
			o.Inheritance,
			winerr.ErrInvalidOption,
		)
	}
	return nil
}
