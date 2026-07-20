package regkey

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// Native is the Windows registry backend.
type Native struct{}

// NewNative returns the live Windows registry backend.
func NewNative() *Native { return &Native{} }

func rootKey(hive string) (registry.Key, error) {
	switch hive {
	case "HKLM":
		return registry.LOCAL_MACHINE, nil
	case "HKCU":
		return registry.CURRENT_USER, nil
	case "HKU":
		return registry.USERS, nil
	case "HKCR":
		return registry.CLASSES_ROOT, nil
	case "HKCC":
		return registry.CURRENT_CONFIG, nil
	default:
		return 0, errs.Wrap("regkey: hive "+hive, errs.ErrInvalidOption)
	}
}

func mapNotFound(op string, err error) error {
	if errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("%s: %w", op, errs.ErrNotFound)
	}
	return fmt.Errorf("%s: %w", op, err)
}

// GetValue implements Backend.
func (n *Native) GetValue(hive, path, name string) (Value, error) {
	root, err := rootKey(hive)
	if err != nil {
		return Value{}, err
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return Value{}, mapNotFound("regkey: opening "+path, err)
	}
	defer k.Close()
	_, kind, err := k.GetValue(name, nil)
	if err != nil {
		return Value{}, mapNotFound("regkey: querying "+name, err)
	}
	switch kind {
	case registry.SZ, registry.EXPAND_SZ:
		s, _, err := k.GetStringValue(name)
		if err != nil {
			return Value{}, mapNotFound("regkey: reading string "+name, err)
		}
		vk := KindString
		if kind == registry.EXPAND_SZ {
			vk = KindExpandString
		}
		return Value{Kind: vk, Data: s}, nil
	case registry.MULTI_SZ:
		s, _, err := k.GetStringsValue(name)
		if err != nil {
			return Value{}, mapNotFound("regkey: reading multi-string "+name, err)
		}
		return Value{Kind: KindMultiString, Data: s}, nil
	case registry.DWORD:
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return Value{}, mapNotFound("regkey: reading dword "+name, err)
		}
		return Value{
			Kind: KindDWord,
			Data: uint32(v),
		}, nil //#nosec G115 -- a REG_DWORD value fits in uint32 by definition
	case registry.QWORD:
		v, _, err := k.GetIntegerValue(name)
		if err != nil {
			return Value{}, mapNotFound("regkey: reading qword "+name, err)
		}
		return Value{Kind: KindQWord, Data: v}, nil
	default:
		b, _, err := k.GetBinaryValue(name)
		if err != nil {
			return Value{}, mapNotFound("regkey: reading binary "+name, err)
		}
		return Value{Kind: KindBinary, Data: b}, nil
	}
}

// SetValue implements Backend.
func (n *Native) SetValue(hive, path, name string, value Value) error {
	root, err := rootKey(hive)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(root, path, registry.SET_VALUE)
	if err != nil {
		return mapNotFound("regkey: creating "+path, err)
	}
	defer k.Close()
	switch value.Kind {
	case KindString:
		err = k.SetStringValue(name, value.Data.(string))
	case KindExpandString:
		err = k.SetExpandStringValue(name, value.Data.(string))
	case KindMultiString:
		err = k.SetStringsValue(name, value.Data.([]string))
	case KindDWord:
		err = k.SetDWordValue(name, value.Data.(uint32))
	case KindQWord:
		err = k.SetQWordValue(name, value.Data.(uint64))
	case KindBinary:
		err = k.SetBinaryValue(name, value.Data.([]byte))
	default:
		return errs.Wrap("regkey: value kind", errs.ErrInvalidOption)
	}
	if err != nil {
		return fmt.Errorf("regkey: setting %s: %w", name, err)
	}
	return nil
}

// DeleteValue implements Backend.
func (n *Native) DeleteValue(hive, path, name string) error {
	root, err := rootKey(hive)
	if err != nil {
		return err
	}
	k, err := registry.OpenKey(root, path, registry.SET_VALUE)
	if err != nil {
		return mapNotFound("regkey: opening "+path, err)
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil {
		return mapNotFound("regkey: deleting value "+name, err)
	}
	return nil
}

// DeleteKey implements Backend.
func (n *Native) DeleteKey(hive, path string, recurse bool) error {
	root, err := rootKey(hive)
	if err != nil {
		return err
	}
	if !recurse {
		if err := registry.DeleteKey(root, path); err != nil {
			return mapNotFound("regkey: deleting "+path, err)
		}
		return nil
	}
	subs, err := n.EnumSubkeys(hive, path)
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return err
	}
	for _, sub := range subs {
		if err := n.DeleteKey(hive, path+`\`+sub, true); err != nil {
			return err
		}
	}
	if err := registry.DeleteKey(root, path); err != nil {
		return mapNotFound("regkey: deleting "+path, err)
	}
	return nil
}

// CreateKey implements Backend.
func (n *Native) CreateKey(hive, path string) error {
	root, err := rootKey(hive)
	if err != nil {
		return err
	}
	k, _, err := registry.CreateKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return mapNotFound("regkey: creating "+path, err)
	}
	return k.Close()
}

// KeyExists implements Backend.
func (n *Native) KeyExists(hive, path string) (bool, error) {
	root, err := rootKey(hive)
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("regkey: opening %s: %w", path, err)
	}
	return true, k.Close()
}

// EnumValues implements Backend.
func (n *Native) EnumValues(hive, path string) (map[string]Value, error) {
	root, err := rootKey(hive)
	if err != nil {
		return nil, err
	}
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, mapNotFound("regkey: opening "+path, err)
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return nil, fmt.Errorf("regkey: enumerating values of %s: %w", path, err)
	}
	out := make(map[string]Value, len(names))
	for _, name := range names {
		v, err := n.GetValue(hive, path, name)
		if err != nil {
			return nil, err
		}
		out[name] = v
	}
	return out, nil
}

// EnumSubkeys implements Backend.
func (n *Native) EnumSubkeys(hive, path string) ([]string, error) {
	root, err := rootKey(hive)
	if err != nil {
		return nil, err
	}
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, mapNotFound("regkey: opening "+path, err)
	}
	defer k.Close()
	subs, err := k.ReadSubKeyNames(-1)
	if err != nil {
		return nil, fmt.Errorf("regkey: enumerating subkeys of %s: %w", path, err)
	}
	return subs, nil
}
