package session

import (
	"errors"
	"fmt"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// deployModeProbes carries the environment probes the Auto deploy-mode
// resolution consults. Every field must be non-nil; Open wires live defaults
// and tests substitute stubs.
type deployModeProbes struct {
	// oobeComplete reports whether the device's Out-of-Box Experience is done.
	oobeComplete func() (bool, error)
	// espUserSetupActive reports whether the Autopilot ESP User Account Setup
	// phase is in progress (wwahost running plus a pending FirstSync flag).
	espUserSetupActive func() (bool, error)
	// sessionZero reports whether the process runs in session 0.
	sessionZero func() bool
	// processInteractive reports whether the process runs on an interactive
	// window station (WinSta0).
	processInteractive func() bool
	// activeUserPresent reports whether any user session is active.
	activeUserPresent func() bool
	// processesToCloseRunning reports whether any AppProcessesToClose entry
	// is currently running.
	processesToCloseRunning func() (bool, error)
}

// resolveAutoDeployMode ports DeploymentSession's Auto-mode decision chain
// (DeploymentSession.cs SetDeploymentProperties): OOBE/ESP force
// NonInteractive, session-0 without an interactive station forces Silent, and
// absent (or not-running) processes-to-close force Silent. The first stage to
// change the mode wins; later stages only log. Callers invoke this only when
// opts.DeployMode is Auto.
//
// Probe errors never abort session open: the affected stage logs a warning
// and leaves the mode unchanged.
func resolveAutoDeployMode(opts Options, p deployModeProbes, log func(msg string)) DeployMode {
	mode := DeployModeAuto
	changed := func() bool { return mode != DeployModeAuto }

	// Stage 1+2: OOBE, then ESP User Account Setup.
	if done, err := p.oobeComplete(); err != nil {
		log(fmt.Sprintf("Failed to determine OOBE completion state, skipping detection: %v", err))
	} else if !done {
		if !opts.NoOobeDetection {
			mode = DeployModeNonInteractive
			log(fmt.Sprintf("Detected OOBE in progress, changing deployment mode to [%s].", mode))
		} else {
			log("Detected OOBE in progress but toolkit is configured to not adjust deployment mode.")
		}
	} else if espActive, err := p.espUserSetupActive(); err != nil {
		log(fmt.Sprintf("Failed to determine ESP state, skipping detection: %v", err))
	} else if espActive {
		if !opts.NoOobeDetection {
			mode = DeployModeNonInteractive
			log(fmt.Sprintf("The ESP User Account Setup phase is still in progress, changing deployment mode to [%s].", mode))
		} else {
			log("The ESP User Account Setup phase is still in progress but toolkit is configured to not adjust deployment mode.")
		}
	} else {
		log("Device has completed the OOBE and toolkit is not running with an active ESP in progress.")
	}

	// Stage 3: session-0 evaluation.
	if p.sessionZero() {
		switch {
		case changed():
			log(fmt.Sprintf("Session 0 detected but deployment has already been changed to [%s].", mode))
		case opts.NoSessionDetection:
			log("Session 0 detected but toolkit is configured to not adjust deployment mode.")
		case opts.ProcessInteractivityDetection && !p.processInteractive():
			mode = DeployModeSilent
			log(fmt.Sprintf("Session 0 detected, process not running in user interactive mode; deployment mode set to [%s].", mode))
		case !p.activeUserPresent():
			if !p.processInteractive() {
				mode = DeployModeSilent
				log(fmt.Sprintf("Session 0 detected, no users logged on and process not running in user interactive mode; deployment mode set to [%s].", mode))
			} else {
				log("Session 0 detected, no users logged on but process running in user interactive mode.")
			}
		default:
			log("Session 0 detected, user(s) logged on to interact if required.")
		}
	} else {
		log("Session 0 not detected, toolkit running as non-SYSTEM user account.")
	}

	// Stage 4: processes-to-close evaluation (PSADT >= 4.2.0 semantics: no
	// specified processes also resolves to Silent).
	if len(opts.AppProcessesToClose) > 0 {
		names := processNames(opts.AppProcessesToClose)
		switch {
		case changed():
			log(fmt.Sprintf("The processes %s were specified as requiring closure but deployment has already been changed to [%s].", names, mode))
		case opts.NoProcessDetection:
			log(fmt.Sprintf("The processes %s were specified as requiring closure but toolkit is configured to not adjust deployment mode.", names))
		default:
			if running, err := p.processesToCloseRunning(); err != nil {
				log(fmt.Sprintf("Failed to evaluate running processes, skipping detection: %v", err))
			} else if !running {
				mode = DeployModeSilent
				log(fmt.Sprintf("The processes %s were specified as requiring closure but none were running, changing deployment mode to [%s].", names, mode))
			} else {
				log(fmt.Sprintf("Processes among %s were found to be running and will require closure.", names))
			}
		}
	} else {
		switch {
		case changed():
			log(fmt.Sprintf("No processes were specified as requiring closure but deployment has already been changed to [%s].", mode))
		case opts.NoProcessDetection:
			log("No processes were specified as requiring closure but toolkit is configured to not adjust deployment mode.")
		default:
			mode = DeployModeSilent
			log(fmt.Sprintf("No processes were specified as requiring closure, changing deployment mode to [%s].", mode))
		}
	}

	if mode == DeployModeAuto {
		mode = DeployModeInteractive
	}
	return mode
}

// processNames renders the close-process names for log lines.
func processNames(procs []ProcessObject) string {
	names := make([]string, len(procs))
	for i, p := range procs {
		names[i] = "'" + p.Name + "'"
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// espEnrollmentsKey is the MDM enrollments root (rooted at HKLM).
const espEnrollmentsKey = `SOFTWARE\Microsoft\Enrollments`

// espFirstSyncPending reports whether an enrollment has a FirstSync entry with
// an unset/zero IsSyncDone flag. When activeUserSID is non-empty the check is
// scoped to that SID's FirstSync entry (PSADT parity); otherwise any pending
// entry counts.
func espFirstSyncPending(reg regkey.Backend, activeUserSID string) (bool, error) {
	enrollments, err := reg.EnumSubkeys("HKLM", espEnrollmentsKey)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	for _, enrollment := range enrollments {
		firstSync := espEnrollmentsKey + `\` + enrollment + `\FirstSync`
		sids, err := reg.EnumSubkeys("HKLM", firstSync)
		if err != nil {
			if errors.Is(err, errs.ErrNotFound) {
				continue
			}
			return false, err
		}
		for _, sid := range sids {
			if activeUserSID != "" && !strings.EqualFold(sid, activeUserSID) {
				continue
			}
			v, err := reg.GetValue("HKLM", firstSync+`\`+sid, "IsSyncDone")
			if err != nil {
				if errors.Is(err, errs.ErrNotFound) {
					return true, nil // no IsSyncDone flag => still syncing
				}
				return false, err
			}
			if syncDone, ok := registryUint(v); ok && syncDone == 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

// registryUint coerces a registry value's data to an unsigned integer.
func registryUint(v regkey.Value) (uint64, bool) {
	switch d := v.Data.(type) {
	case uint32:
		return uint64(d), true
	case uint64:
		return d, true
	case int:
		if d >= 0 {
			return uint64(d), true
		}
	case int64:
		if d >= 0 {
			return uint64(d), true
		}
	}
	return 0, false
}

// userUICulture reads the active user's Windows display language from
// HKU\<sid>\Control Panel\International\User Profile value "Languages"
// (REG_MULTI_SZ; the first entry, e.g. "en-GB"). Returns "" when unavailable.
func userUICulture(reg regkey.Backend, sid string) string {
	if sid == "" {
		return ""
	}
	v, err := reg.GetValue("HKU", sid+`\Control Panel\International\User Profile`, "Languages")
	if err != nil {
		return ""
	}
	switch d := v.Data.(type) {
	case []string:
		if len(d) > 0 {
			return strings.TrimSpace(d[0])
		}
	case string:
		return strings.TrimSpace(d)
	}
	return ""
}
