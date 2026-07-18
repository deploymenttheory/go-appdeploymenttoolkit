//go:build !windows

package psadt

import "github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"

// The User and Machine environment stores are Windows registry locations;
// only the Process target works elsewhere.

func getEnvironmentVariableFromRegistry(
	_ string,
	_ EnvironmentVariableTarget,
) (string, error) {
	return "", winerr.Wrap("psadt: user/machine environment variables", winerr.ErrNotWindows)
}

func setEnvironmentVariableInRegistry(
	_, _ string,
	_ EnvironmentVariableTarget,
	_ bool,
) error {
	return winerr.Wrap("psadt: user/machine environment variables", winerr.ErrNotWindows)
}

func removeEnvironmentVariableFromRegistry(_ string, _ EnvironmentVariableTarget) error {
	return winerr.Wrap("psadt: user/machine environment variables", winerr.ErrNotWindows)
}
