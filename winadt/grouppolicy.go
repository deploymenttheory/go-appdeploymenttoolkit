package winadt

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
)

// UpdateADTGroupPolicy is the Go port of Update-ADTGroupPolicy: it refreshes
// both the Computer and User Group Policy settings on the local machine by
// running gpupdate.exe with /Force for each target.
//
// Deviation from PSADT: PSADT runs the User target inside the active user's
// session (Start-ADTProcess -RunAsActiveUser); this port runs gpupdate.exe in
// the current session for both targets. Failures from either target are
// collected and returned together.
func UpdateADTGroupPolicy(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: UpdateADTGroupPolicy: %w", err)
	}
	var errs []error
	for _, target := range []string{"Computer", "User"} {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("adt: UpdateADTGroupPolicy: %w", err))
			break
		}
		logToSession(fmt.Sprintf("Updating Group Policies for the %s.", target),
			LogSeverityInfo, "UpdateADTGroupPolicy")
		if _, err := StartADTProcess(ctx, StartADTProcessOptions{
			FilePath:       gpupdateExePath(),
			ArgumentList:   buildGpUpdateArgs(target, true),
			CreateNoWindow: true,
		}); err != nil {
			errs = append(errs, fmt.Errorf("adt: updating Group Policy for the %s: %w", target, err))
		}
	}
	return errors.Join(errs...)
}

// buildGpUpdateArgs composes the gpupdate.exe argument string for a target,
// mirroring Update-ADTGroupPolicy ("/Target:Computer" plus "/Force").
func buildGpUpdateArgs(target string, force bool) string {
	args := "/Target:" + target
	if force {
		args += " /Force"
	}
	return args
}

// gpupdateExePath returns %WINDIR%\System32\gpupdate.exe, matching PSADT's use
// of the system directory.
func gpupdateExePath() string {
	if windir := windowsDir(); windir != "" {
		return filepath.Join(windir, "System32", "gpupdate.exe")
	}
	return "gpupdate.exe"
}
