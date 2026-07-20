package shortcut

import (
	"fmt"
	"strings"
	"unsafe"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	systemcom "github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/shell"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// CLSID_ShellLink is the shell-link coclass {00021401-0000-0000-C000-000000000046}.
var clsidShellLink = win32.GUID{Data1: 0x00021401, Data4: [8]byte{0xc0, 0, 0, 0, 0, 0, 0, 0x46}}

// withShellLink runs fn with an initialized IShellLinkW + IPersistFile pair.
func withShellLink(fn func(link *shell.IShellLinkW, file *systemcom.IPersistFile) error) error {
	if _, err := systemcom.CoInitializeEx(uint32(systemcom.COINIT_APARTMENTTHREADED)); err != nil {
		return fmt.Errorf("shortcut: CoInitializeEx: %w", err)
	}
	defer systemcom.CoUninitialize()

	var unk *win32.IUnknown
	if err := systemcom.CoCreateInstance(
		&clsidShellLink,
		nil,
		systemcom.CLSCTX_INPROC_SERVER,
		&shell.IID_IShellLinkW,
		&unk,
	); err != nil {
		return fmt.Errorf("shortcut: CoCreateInstance(ShellLink): %w", err)
	}
	link := (*shell.IShellLinkW)(unsafe.Pointer(unk))
	defer link.Release()

	var fileUnk *win32.IUnknown
	if err := unk.QueryInterface(&systemcom.IID_IPersistFile, &fileUnk); err != nil {
		return fmt.Errorf("shortcut: QueryInterface(IPersistFile): %w", err)
	}
	file := (*systemcom.IPersistFile)(unsafe.Pointer(fileUnk))
	defer file.Release()

	return fn(link, file)
}

// writeLinkFile creates or overwrites a .lnk file from the model.
func writeLinkFile(s *Shortcut) error {
	return withShellLink(func(link *shell.IShellLinkW, file *systemcom.IPersistFile) error {
		if err := link.SetPath(s.TargetPath); err != nil {
			return fmt.Errorf("shortcut: SetPath: %w", err)
		}
		if s.Arguments != "" {
			if err := link.SetArguments(s.Arguments); err != nil {
				return fmt.Errorf("shortcut: SetArguments: %w", err)
			}
		}
		if s.Description != "" {
			if err := link.SetDescription(s.Description); err != nil {
				return fmt.Errorf("shortcut: SetDescription: %w", err)
			}
		}
		if s.WorkingDirectory != "" {
			if err := link.SetWorkingDirectory(s.WorkingDirectory); err != nil {
				return fmt.Errorf("shortcut: SetWorkingDirectory: %w", err)
			}
		}
		if s.IconLocation != "" {
			if err := link.SetIconLocation(s.IconLocation, iconIndex32(s.IconIndex)); err != nil {
				return fmt.Errorf("shortcut: SetIconLocation: %w", err)
			}
		}
		style := s.WindowStyle
		if style == 0 {
			style = WindowStyleNormal
		}
		if err := link.SetShowCmd(windowsandmessaging.SHOW_WINDOW_CMD(style)); err != nil {
			return fmt.Errorf("shortcut: SetShowCmd: %w", err)
		}
		if s.Hotkey != "" {
			hk, err := parseHotkey(s.Hotkey)
			if err != nil {
				return err
			}
			if err := link.SetHotkey(hk); err != nil {
				return fmt.Errorf("shortcut: SetHotkey: %w", err)
			}
		}
		if err := file.Save(s.Path, true); err != nil {
			return fmt.Errorf("shortcut: IPersistFile.Save: %w", err)
		}
		return nil
	})
}

// readLinkFile loads a .lnk file into the model.
func readLinkFile(path string) (*Shortcut, error) {
	s := &Shortcut{Path: path}
	err := withShellLink(func(link *shell.IShellLinkW, file *systemcom.IPersistFile) error {
		if err := file.Load(path, systemcom.STGM_READ); err != nil {
			return fmt.Errorf("shortcut: IPersistFile.Load: %w", err)
		}
		const bufLen int32 = 1024
		buf := make([]uint16, bufLen)
		if err := link.GetPath(
			foundation.PWSTR(unsafe.SliceData(buf)),
			bufLen,
			nil,
			0,
		); err == nil {
			s.TargetPath = win32.UTF16ToString(unsafe.SliceData(buf))
		}
		if err := link.GetArguments(
			foundation.PWSTR(unsafe.SliceData(buf)),
			bufLen,
		); err == nil {
			s.Arguments = win32.UTF16ToString(unsafe.SliceData(buf))
		}
		if err := link.GetDescription(
			foundation.PWSTR(unsafe.SliceData(buf)),
			bufLen,
		); err == nil {
			s.Description = win32.UTF16ToString(unsafe.SliceData(buf))
		}
		if err := link.GetWorkingDirectory(
			foundation.PWSTR(unsafe.SliceData(buf)),
			bufLen,
		); err == nil {
			s.WorkingDirectory = win32.UTF16ToString(unsafe.SliceData(buf))
		}
		var iconIndex int32
		if err := link.GetIconLocation(
			foundation.PWSTR(unsafe.SliceData(buf)),
			bufLen,
			&iconIndex,
		); err == nil {
			s.IconLocation = win32.UTF16ToString(unsafe.SliceData(buf))
			s.IconIndex = int(iconIndex)
		}
		var show windowsandmessaging.SHOW_WINDOW_CMD
		if err := link.GetShowCmd(&show); err == nil {
			s.WindowStyle = WindowStyle(show)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

// hotkey modifier flags (HOTKEYF_* << 8).
const (
	hotkeyShift = 0x0100
	hotkeyCtrl  = 0x0200
	hotkeyAlt   = 0x0400
)

// parseHotkey converts PSADT-style hotkey strings ("Ctrl+Shift+F9") into the
// IShellLink hotkey word.
func parseHotkey(s string) (uint16, error) {
	var mods uint16
	var key uint16
	for _, part := range strings.Split(s, "+") {
		p := strings.TrimSpace(part)
		switch {
		case strings.EqualFold(p, "ctrl"), strings.EqualFold(p, "control"):
			mods |= hotkeyCtrl
		case strings.EqualFold(p, "shift"):
			mods |= hotkeyShift
		case strings.EqualFold(p, "alt"):
			mods |= hotkeyAlt
		default:
			vk, err := virtualKey(p)
			if err != nil {
				return 0, err
			}
			key = vk
		}
	}
	return mods | key, nil
}

// virtualKey maps single characters and F-keys to virtual-key codes.
func virtualKey(p string) (uint16, error) {
	upper := strings.ToUpper(p)
	if len(upper) == 1 {
		c := upper[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint16(c), nil
		}
	}
	if strings.HasPrefix(upper, "F") && len(upper) <= 3 {
		var n int
		if _, err := fmt.Sscanf(upper[1:], "%d", &n); err == nil && n >= 1 && n <= 24 {
			return uint16(0x70 + n - 1), nil // VK_F1 = 0x70
		}
	}
	return 0, winerr.Wrap("shortcut: hotkey key "+p, winerr.ErrInvalidOption)
}

// iconIndex32 bounds an icon index into int32 range (icon indices are tiny;
// this exists to satisfy overflow-safety linting).
func iconIndex32(i int) int32 {
	const maxIconIndex = 1 << 30
	if i > maxIconIndex || i < -maxIconIndex {
		return 0
	}
	return int32(i)
}
