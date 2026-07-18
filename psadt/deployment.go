package psadt

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
)

// PhaseFunc is one deployment phase (the Go analogue of the Pre-Install /
// Install / Post-Install scriptblocks of Invoke-AppDeployToolkit.ps1).
type PhaseFunc func(ctx context.Context, s *DeploymentSession) error

// Deployment is the Go analogue of an Invoke-AppDeployToolkit.ps1 frontend
// script: session metadata plus the phase functions per deployment type.
// Nil phases are skipped.
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

// Run parses the standard PSADT frontend flags (-DeploymentType,
// -DeployMode, -SuppressRebootPassThru), opens the session, dispatches the
// three phase functions for the resolved deployment type (setting
// InstallPhase around each), recovers panics into a logged fatal exit, then
// closes the session and exits the process with the final exit code.
func (d *Deployment) Run(ctx context.Context) {
	exit := d.Exit
	if exit == nil {
		exit = os.Exit
	}
	exit(d.run(ctx))
}

func (d *Deployment) run(ctx context.Context) int {
	opts := d.Session
	if err := d.parseFlags(&opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return ExitCodeRunnerFailure
	}

	s, err := OpenADTSession(ctx, opts)
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
	return CloseADTSession(ctx, s)
}

func (d *Deployment) parseFlags(opts *SessionOptions) error {
	args := d.Args
	if args == nil {
		args = os.Args[1:]
	}
	fs := flag.NewFlagSet("psadt", flag.ContinueOnError)
	deploymentType := fs.String("DeploymentType", "", "Install, Uninstall or Repair")
	deployMode := fs.String("DeployMode", "", "Auto, Interactive, NonInteractive or Silent")
	suppressReboot := fs.Bool("SuppressRebootPassThru", false, "return 0 instead of a reboot exit code")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("psadt: parsing frontend flags: %w", err)
	}
	if *deploymentType != "" {
		t, ok := session.ParseDeploymentType(*deploymentType)
		if !ok {
			return fmt.Errorf("psadt: -DeploymentType %q: %w", *deploymentType, ErrInvalidOption)
		}
		opts.DeploymentType = t
	}
	if *deployMode != "" {
		m, ok := session.ParseDeployMode(*deployMode)
		if !ok {
			return fmt.Errorf("psadt: -DeployMode %q: %w", *deployMode, ErrInvalidOption)
		}
		opts.DeployMode = m
	}
	if *suppressReboot {
		opts.SuppressRebootPassThru = true
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

func runPhase(ctx context.Context, s *DeploymentSession, name string, fn PhaseFunc) (err error) {
	if fn == nil {
		return nil
	}
	s.SetInstallPhase(name)
	defer func() {
		if r := recover(); r != nil {
			s.WriteLog(fmt.Sprintf("Panic in %s phase: %v", name, r), LogSeverityError, "Deployment.Run", "")
			err = NewExitError(ExitCodeRunnerFailure, fmt.Errorf("psadt: panic in %s phase", name))
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
func applyPhaseError(s *DeploymentSession, err error) {
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
