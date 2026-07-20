package manifest

import (
	"context"
	"encoding/base64"
	"math"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

// registryKindEnum lists the manifest names for RegistryValueKind.
var registryKindEnum = []string{"string", "expandString", "multiString", "binary", "dword", "qword"}

// registryKindFromString maps a manifest kind name onto RegistryValueKind
// (empty = inferred from the value's Go type).
func registryKindFromString(s string) adt.RegistryValueKind {
	switch strings.ToLower(s) {
	case "string":
		return adt.RegistryValueKindString
	case "expandstring":
		return adt.RegistryValueKindExpandString
	case "multistring":
		return adt.RegistryValueKindMultiString
	case "binary":
		return adt.RegistryValueKindBinary
	case "dword":
		return adt.RegistryValueKindDWord
	case "qword":
		return adt.RegistryValueKindQWord
	default:
		return adt.RegistryValueKindInferred
	}
}

func init() {
	register(StepSpec{
		Name: "registry.set", Summary: "Create a registry key or set a value",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "key", Type: TypeString, Required: true, Description: `registry key path, e.g. HKLM:\SOFTWARE\Contoso`},
			{Name: "name", Type: TypeString, Description: `value name; absent only creates the key ("(Default)" for the default value)`},
			{Name: "value", Type: TypeAny, Description: "value data; YAML typing decides string vs int, `type` disambiguates, binary is base64"},
			{Name: "type", Type: TypeString, Enum: registryKindEnum,
				Description: "force the registry value kind (default inferred)"},
			{Name: "wow6432Node", Type: TypeBool, Description: "write the 32-bit registry view"},
		},
		Check: func(p Params, add AddIssue) {
			kind, _ := p.String("type")
			val, hasVal := p.Any("value")
			switch strings.ToLower(kind) {
			case "multistring":
				if _, ok := val.([]string); hasVal && !ok {
					add(CodeSemantic, "value", "multiString values must be a list of strings", false)
				}
			case "binary":
				s, ok := val.(string)
				if hasVal && ok {
					if _, err := base64.StdEncoding.DecodeString(s); err != nil {
						add(CodeSemantic, "value", "binary values must be base64-encoded", false)
					}
				} else if hasVal {
					add(CodeSemantic, "value", "binary values must be a base64 string", false)
				}
			case "dword":
				if i, ok := val.(int); hasVal && (!ok || i < 0 || int64(i) > math.MaxUint32) {
					add(CodeSemantic, "value", "dword values must be integers in the uint32 range", false)
				}
			case "qword":
				if _, ok := val.(int); hasVal && !ok {
					add(CodeSemantic, "value", "qword values must be integers", false)
				}
			}
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.SetADTRegistryKeyOptions{
				Key:         p.StringOr("key", ""),
				Name:        p.StringOr("name", ""),
				Type:        registryKindFromString(p.StringOr("type", "")),
				Wow6432Node: p.BoolOr("wow6432Node", false),
			}
			if v, ok := p.Any("value"); ok {
				opts.Value = v
				if opts.Type == adt.RegistryValueKindBinary {
					if s, isStr := v.(string); isStr {
						if raw, err := base64.StdEncoding.DecodeString(s); err == nil {
							opts.Value = raw
						}
					}
				}
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.SetADTRegistryKey(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "registry.remove", Summary: "Remove a registry key or value",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "key", Type: TypeString, Required: true, Description: "registry key path"},
			{Name: "name", Type: TypeString, Description: "value to delete; absent deletes the key itself"},
			{Name: "recurse", Type: TypeBool, Description: "delete the key's subtree"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.RemoveADTRegistryKeyOptions{
				Key:     p.StringOr("key", ""),
				Name:    p.StringOr("name", ""),
				Recurse: p.BoolOr("recurse", false),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.RemoveADTRegistryKey(ctx, opts)
			}, nil
		},
	})
}
