//go:build !windows

package winadt

import "github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"

// The User and Machine environment stores are Windows registry locations;
// only the Process target works elsewhere.

func getEnvironmentVariableFromRegistry(
	_ string,
	_ EnvironmentVariableTarget,
) (string, error) {
	return "", winerr.Wrap("adt: user/machine environment variables", winerr.ErrNotWindows)
}

func setEnvironmentVariableInRegistry(
	_, _ string,
	_ EnvironmentVariableTarget,
	_ bool,
) error {
	return winerr.Wrap("adt: user/machine environment variables", winerr.ErrNotWindows)
}

func removeEnvironmentVariableFromRegistry(_ string, _ EnvironmentVariableTarget) error {
	return winerr.Wrap("adt: user/machine environment variables", winerr.ErrNotWindows)
}
