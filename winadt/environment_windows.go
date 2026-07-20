package winadt

import (
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// machineEnvironmentKeyPath is the HKLM store behind Machine-target
// environment variables.
const machineEnvironmentKeyPath = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

// environmentKey opens the registry key backing the User or Machine store.
func environmentKey(target EnvironmentVariableTarget, access uint32) (registry.Key, error) {
	if target == EnvironmentTargetMachine {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, machineEnvironmentKeyPath, access)
		if err != nil {
			return 0, fmt.Errorf("adt: opening machine environment key: %w", err)
		}
		return k, nil
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, access)
	if err != nil {
		return 0, fmt.Errorf("adt: opening user environment key: %w", err)
	}
	return k, nil
}

// getEnvironmentVariableFromRegistry reads a User/Machine variable.
func getEnvironmentVariableFromRegistry(
	variable string,
	target EnvironmentVariableTarget,
) (string, error) {
	k, err := environmentKey(target, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer k.Close() //nolint:errcheck // read-only handle
	value, _, err := k.GetStringValue(variable)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return "", fmt.Errorf("adt: environment variable %s (%s): %w",
				variable, target, winerr.ErrNotFound)
		}
		return "", fmt.Errorf("adt: reading environment variable %s (%s): %w",
			variable, target, err)
	}
	return value, nil
}

// setEnvironmentVariableInRegistry writes a User/Machine variable and
// broadcasts the change.
func setEnvironmentVariableInRegistry(
	variable, value string,
	target EnvironmentVariableTarget,
	expandable bool,
) error {
	k, err := environmentKey(target, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close() //nolint:errcheck // value write is checked below
	if expandable {
		err = k.SetExpandStringValue(variable, value)
	} else {
		err = k.SetStringValue(variable, value)
	}
	if err != nil {
		return fmt.Errorf("adt: writing environment variable %s (%s): %w", variable, target, err)
	}
	broadcastEnvironmentChange()
	return nil
}

// removeEnvironmentVariableFromRegistry deletes a User/Machine variable
// (idempotent) and broadcasts the change.
func removeEnvironmentVariableFromRegistry(
	variable string,
	target EnvironmentVariableTarget,
) error {
	k, err := environmentKey(target, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close() //nolint:errcheck // value delete is checked below
	if err := k.DeleteValue(variable); err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil // already absent
		}
		return fmt.Errorf("adt: deleting environment variable %s (%s): %w", variable, target, err)
	}
	broadcastEnvironmentChange()
	return nil
}

// broadcastEnvironmentChange notifies top-level windows that the environment
// block changed (WM_SETTINGCHANGE "Environment"), best-effort.
func broadcastEnvironmentChange() {
	const hwndBroadcast = foundation.HWND(0xffff)
	env, err := windows.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	//nolint:errcheck // best-effort notification; failures are inconsequential
	_, _ = windowsandmessaging.SendMessageTimeout(
		hwndBroadcast,
		windowsandmessaging.WM_SETTINGCHANGE,
		0,
		foundation.LPARAM(uintptr(unsafe.Pointer(env))),
		windowsandmessaging.SMTO_ABORTIFHUNG,
		5000,
		&result,
	)
	runtime.KeepAlive(env)
}
