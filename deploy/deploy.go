package deploy

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// PhaseFunc is one deployment phase (the Go analogue of the Pre-Install /
// Install / Post-Install scriptblocks of PSADT's frontend script).
type PhaseFunc func(ctx context.Context, s *Session) error

// Deployment is the engine-level deployment description: session metadata
// plus the phase functions per deployment type. Nil phases are skipped.
type Deployment struct {
	Session SessionOptions

	PreInstall  PhaseFunc
	Install     PhaseFunc
	PostInstall PhaseFunc

	PreUninstall  PhaseFunc
	Uninstall     PhaseFunc
	PostUninstall PhaseFunc

	PreRepair  PhaseFunc
	Repair     PhaseFunc
	PostRepair PhaseFunc

	// Args overrides os.Args[1:] for flag parsing (used by tests and the
	// CLI runner). Nil means os.Args[1:].
	Args []string

	// Exit overrides os.Exit (used by tests). Nil means os.Exit.
	Exit func(code int)
}

// Run parses the standard frontend flags (-DeploymentType, -DeployMode,
// -SuppressRebootPassThru, -NoOobeDetection, -NoProcessDetection,
// -NoSessionDetection, -ProcessInteractivityDetection), opens the session,
// dispatches the three phase functions for the resolved deployment type
// (setting InstallPhase around each), recovers panics into a logged fatal
// exit, then closes the session and exits the process with the final exit
// code.
func (d *Deployment) Run(ctx context.Context) {
	exit := d.Exit
	if exit == nil {
		exit = os.Exit
	}
	exit(d.run(ctx))
}

func (d *Deployment) run(ctx context.Context) int {
	opts := d.Session
	// A compiled deployment binary sits at its package root beside Files/ and
	// SupportFiles/. When the author leaves ScriptDirectory unset, default it
	// to the executable's directory so relative installer paths resolve (the
	// session only derives DirFiles/DirSupportFiles from a non-empty
	// ScriptDirectory). The CLI runner sets ScriptDirectory explicitly, so this
	// only affects standalone deployments.
	if opts.ScriptDirectory == "" {
		if exe, err := os.Executable(); err == nil {
			opts.ScriptDirectory = filepath.Dir(exe)
		} else {
			opts.ScriptDirectory = "."
		}
	}
	if err := d.parseFlags(&opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeRunnerFailure
	}

	s, err := Open(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to open deployment session: %v\n", err)
		return ExitCodeRunnerFailure
	}

	pre, main, post := d.phases(opts.DeploymentType)
	typeName := opts.DeploymentType.String()

	err = runPhase(ctx, s, "Pre-"+typeName, pre)
	if err == nil {
		err = runPhase(ctx, s, typeName, main)
	}
	if err == nil {
		err = runPhase(ctx, s, "Post-"+typeName, post)
	}
	if err != nil {
		applyPhaseError(s, err)
	}
	return Close(ctx, s)
}

func (d *Deployment) parseFlags(opts *SessionOptions) error {
	args := d.Args
	if args == nil {
		args = os.Args[1:]
	}
	fs := flag.NewFlagSet("adt", flag.ContinueOnError)
	deploymentType := fs.String("DeploymentType", "", "Install, Uninstall or Repair")
	deployMode := fs.String("DeployMode", "", "Auto, Interactive, NonInteractive or Silent")
	suppressReboot := fs.Bool("SuppressRebootPassThru", false, "return 0 instead of a reboot exit code")
	noOobe := fs.Bool("NoOobeDetection", false, "skip OOBE/ESP checks during Auto deploy-mode resolution")
	noProcess := fs.Bool("NoProcessDetection", false, "skip processes-to-close checks during Auto deploy-mode resolution")
	noSession := fs.Bool("NoSessionDetection", false, "skip session-0 checks during Auto deploy-mode resolution")
	interactivity := fs.Bool("ProcessInteractivityDetection", false, "in session 0, require an interactive window station")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("adt: parsing frontend flags: %w", err)
	}
	if *deploymentType != "" {
		t, ok := ParseDeploymentType(*deploymentType)
		if !ok {
			return fmt.Errorf("adt: -DeploymentType %q: %w", *deploymentType, ErrInvalidOption)
		}
		opts.DeploymentType = t
	}
	if *deployMode != "" {
		m, ok := ParseDeployMode(*deployMode)
		if !ok {
			return fmt.Errorf("adt: -DeployMode %q: %w", *deployMode, ErrInvalidOption)
		}
		opts.DeployMode = m
	}
	if *suppressReboot {
		opts.SuppressRebootPassThru = true
	}
	if *noOobe {
		opts.NoOobeDetection = true
	}
	if *noProcess {
		opts.NoProcessDetection = true
	}
	if *noSession {
		opts.NoSessionDetection = true
	}
	if *interactivity {
		opts.ProcessInteractivityDetection = true
	}
	return nil
}

func (d *Deployment) phases(t DeploymentType) (pre, main, post PhaseFunc) {
	switch t {
	case DeploymentTypeUninstall:
		return d.PreUninstall, d.Uninstall, d.PostUninstall
	case DeploymentTypeRepair:
		return d.PreRepair, d.Repair, d.PostRepair
	default:
		return d.PreInstall, d.Install, d.PostInstall
	}
}

func runPhase(ctx context.Context, s *Session, name string, fn PhaseFunc) (err error) {
	if fn == nil {
		return nil
	}
	s.SetInstallPhase(name)
	defer func() {
		if r := recover(); r != nil {
			s.WriteLog(fmt.Sprintf("Panic in %s phase: %v", name, r), LogSeverityError, "Deployment.Run", "")
			err = NewExitError(ExitCodeRunnerFailure, fmt.Errorf("adt: panic in %s phase", name))
		}
	}()
	if err := fn(ctx, s); err != nil {
		return err
	}
	return nil
}

// applyPhaseError maps a phase error to the session exit code, mirroring the
// PSADT frontend's catch block: deferral maps to the configured defer exit
// code, an ExitError carries its own code, anything else is a generic
// failure (60001).
func applyPhaseError(s *Session, err error) {
	switch {
	case errors.Is(err, ErrDeferred):
		s.SetExitCode(s.Config().UI.DeferExitCode)
	case errors.Is(err, ErrTimeout):
		s.SetExitCode(s.Config().UI.DefaultExitCode)
	default:
		if ee, ok := AsExitError(err); ok {
			s.SetExitCode(ee.Code)
		} else if s.ExitCode() == 0 {
			s.SetExitCode(ExitCodeGenericFailure)
		}
	}
	if !errors.Is(err, ErrDeferred) {
		s.WriteLog(fmt.Sprintf("Deployment phase failed: %v", err), LogSeverityError, "Deployment.Run", "")
	}
}
