// Package svcmgmt wraps the Windows Service Control Manager operations the
// toolkit needs: existence checks, start-mode management, and starting or
// stopping services together with their dependents.
package svcmgmt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// StartMode mirrors PSADT's service start-mode names.
type StartMode string

// StartMode values.
const (
	StartModeAutomatic        StartMode = "Automatic"
	StartModeAutomaticDelayed StartMode = "Automatic (Delayed Start)"
	StartModeManual           StartMode = "Manual"
	StartModeDisabled         StartMode = "Disabled"
)

func withService(name string, access uint32, fn func(m *mgr.Mgr, s *mgr.Service) error) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("svcmgmt: connecting to SCM: %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	handle, err := windows.OpenService(m.Handle, windows.StringToUTF16Ptr(name), access)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return fmt.Errorf("svcmgmt: service %s: %w", name, winerr.ErrNotFound)
		}
		return fmt.Errorf("svcmgmt: opening service %s: %w", name, err)
	}
	s := &mgr.Service{Name: name, Handle: handle}
	defer func() { _ = s.Close() }()
	return fn(m, s)
}

// Exists reports whether the named service is installed.
func Exists(name string) (bool, error) {
	err := withService(
		name,
		windows.SERVICE_QUERY_STATUS,
		func(*mgr.Mgr, *mgr.Service) error { return nil },
	)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, winerr.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// GetStartMode returns the service's start mode, distinguishing delayed
// automatic start like PSADT does.
func GetStartMode(name string) (StartMode, error) {
	var mode StartMode
	err := withService(name, windows.SERVICE_QUERY_CONFIG, func(_ *mgr.Mgr, s *mgr.Service) error {
		cfg, err := s.Config()
		if err != nil {
			return fmt.Errorf("svcmgmt: querying config of %s: %w", name, err)
		}
		switch cfg.StartType {
		case mgr.StartAutomatic:
			mode = StartModeAutomatic
			if cfg.DelayedAutoStart {
				mode = StartModeAutomaticDelayed
			}
		case mgr.StartManual:
			mode = StartModeManual
		case mgr.StartDisabled:
			mode = StartModeDisabled
		default:
			mode = StartMode(fmt.Sprintf("Unknown (%d)", cfg.StartType))
		}
		return nil
	})
	return mode, err
}

// SetStartMode sets the service start mode (parity with Set-ADTServiceStartMode).
func SetStartMode(name string, mode StartMode) error {
	return withService(
		name,
		windows.SERVICE_CHANGE_CONFIG|windows.SERVICE_QUERY_CONFIG,
		func(_ *mgr.Mgr, s *mgr.Service) error {
			cfg, err := s.Config()
			if err != nil {
				return fmt.Errorf("svcmgmt: querying config of %s: %w", name, err)
			}
			switch mode {
			case StartModeAutomatic:
				cfg.StartType, cfg.DelayedAutoStart = mgr.StartAutomatic, false
			case StartModeAutomaticDelayed:
				cfg.StartType, cfg.DelayedAutoStart = mgr.StartAutomatic, true
			case StartModeManual:
				cfg.StartType, cfg.DelayedAutoStart = mgr.StartManual, false
			case StartModeDisabled:
				cfg.StartType, cfg.DelayedAutoStart = mgr.StartDisabled, false
			default:
				return winerr.Wrap("svcmgmt: start mode "+string(mode), winerr.ErrInvalidOption)
			}
			if err := s.UpdateConfig(cfg); err != nil {
				return fmt.Errorf("svcmgmt: updating config of %s: %w", name, err)
			}
			return nil
		},
	)
}

// ParseStartMode validates a PSADT start-mode string.
func ParseStartMode(s string) (StartMode, error) {
	for _, m := range []StartMode{StartModeAutomatic, StartModeAutomaticDelayed, StartModeManual, StartModeDisabled} {
		if strings.EqualFold(string(m), s) {
			return m, nil
		}
	}
	return "", winerr.Wrap("svcmgmt: start mode "+s, winerr.ErrInvalidOption)
}

func waitForState(
	ctx context.Context,
	s *mgr.Service,
	want svc.State,
	timeout time.Duration,
) error {
	deadline := time.Now().Add(timeout)
	for {
		status, err := s.Query()
		if err != nil {
			return fmt.Errorf("svcmgmt: querying status: %w", err)
		}
		if status.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("svcmgmt: waiting for service state %d: %w", want, winerr.ErrTimeout)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("svcmgmt: %w", ctx.Err())
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// Start starts the named service and waits until it is running.
func Start(ctx context.Context, name string, timeout time.Duration) error {
	return withService(
		name,
		windows.SERVICE_START|windows.SERVICE_QUERY_STATUS,
		func(_ *mgr.Mgr, s *mgr.Service) error {
			status, err := s.Query()
			if err == nil && status.State == svc.Running {
				return nil
			}
			if err := s.Start(); err != nil {
				return fmt.Errorf("svcmgmt: starting %s: %w", name, err)
			}
			return waitForState(ctx, s, svc.Running, timeout)
		},
	)
}

// Stop stops the named service (and, when stopDependents, its running
// dependent services first) and waits until stopped.
func Stop(ctx context.Context, name string, stopDependents bool, timeout time.Duration) error {
	if stopDependents {
		deps, err := dependentServices(name)
		if err != nil {
			return err
		}
		for _, dep := range deps {
			if err := Stop(
				ctx,
				dep,
				true,
				timeout,
			); err != nil &&
				!errors.Is(err, winerr.ErrNotFound) {
				return err
			}
		}
	}
	return withService(
		name,
		windows.SERVICE_STOP|windows.SERVICE_QUERY_STATUS,
		func(_ *mgr.Mgr, s *mgr.Service) error {
			status, err := s.Query()
			if err == nil && status.State == svc.Stopped {
				return nil
			}
			if _, err := s.Control(svc.Stop); err != nil {
				return fmt.Errorf("svcmgmt: stopping %s: %w", name, err)
			}
			return waitForState(ctx, s, svc.Stopped, timeout)
		},
	)
}

// StartWithDependencies starts the service, then restarts any dependent
// services that were running before (parity with Start-ADTServiceAndDependencies).
func StartWithDependencies(ctx context.Context, name string, timeout time.Duration) error {
	if err := Start(ctx, name, timeout); err != nil {
		return err
	}
	deps, err := dependentServices(name)
	if err != nil {
		return err
	}
	for _, dep := range deps {
		if err := Start(ctx, dep, timeout); err != nil && !errors.Is(err, winerr.ErrNotFound) {
			return err
		}
	}
	return nil
}

// dependentServices lists services that depend on name (active only).
func dependentServices(name string) ([]string, error) {
	var deps []string
	err := withService(
		name,
		windows.SERVICE_ENUMERATE_DEPENDENTS,
		func(_ *mgr.Mgr, s *mgr.Service) error {
			list, err := s.ListDependentServices(svc.Active)
			if err != nil {
				return fmt.Errorf("svcmgmt: listing dependents of %s: %w", name, err)
			}
			deps = list
			return nil
		},
	)
	return deps, err
}
