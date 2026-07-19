package adt

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// EnvironmentVariableTarget mirrors System.EnvironmentVariableTarget.
type EnvironmentVariableTarget string

// EnvironmentVariableTarget values.
const (
	// EnvironmentTargetProcess reads/writes the current process block.
	EnvironmentTargetProcess EnvironmentVariableTarget = "Process"
	// EnvironmentTargetUser is backed by HKCU\Environment (Windows-only).
	EnvironmentTargetUser EnvironmentVariableTarget = "User"
	// EnvironmentTargetMachine is backed by HKLM\SYSTEM\CurrentControlSet\
	// Control\Session Manager\Environment (Windows-only).
	EnvironmentTargetMachine EnvironmentVariableTarget = "Machine"
)

// parseEnvironmentTarget normalizes the target string; empty means Process.
func parseEnvironmentTarget(s string) (EnvironmentVariableTarget, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "process":
		return EnvironmentTargetProcess, nil
	case "user":
		return EnvironmentTargetUser, nil
	case "machine":
		return EnvironmentTargetMachine, nil
	default:
		return "", fmt.Errorf("adt: unknown environment variable target %q: %w",
			s, winerr.ErrInvalidOption)
	}
}

// GetADTEnvironmentVariableOptions mirrors Get-ADTEnvironmentVariable.
type GetADTEnvironmentVariableOptions struct {
	// Variable is the environment variable name.
	Variable string
	// Target is Process (default), User or Machine.
	Target string
}

// GetADTEnvironmentVariable is the Go port of Get-ADTEnvironmentVariable: it
// reads an environment variable from the process block or, on Windows, from
// the User/Machine registry stores. Missing variables return
// winerr.ErrNotFound.
func GetADTEnvironmentVariable(
	ctx context.Context,
	opts GetADTEnvironmentVariableOptions,
) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	if strings.TrimSpace(opts.Variable) == "" {
		return "", winerr.Wrap("GetADTEnvironmentVariable: a variable name is required",
			winerr.ErrInvalidOption)
	}
	target, err := parseEnvironmentTarget(opts.Target)
	if err != nil {
		return "", err
	}
	logToSession(
		fmt.Sprintf("Getting the environment variable [%s] for [%s].", opts.Variable, target),
		LogSeverityInfo, "Get-ADTEnvironmentVariable",
	)
	if target == EnvironmentTargetProcess {
		value, ok := os.LookupEnv(opts.Variable)
		if !ok {
			return "", fmt.Errorf("adt: environment variable %s: %w",
				opts.Variable, winerr.ErrNotFound)
		}
		return value, nil
	}
	return getEnvironmentVariableFromRegistry(opts.Variable, target)
}

// SetADTEnvironmentVariableOptions mirrors Set-ADTEnvironmentVariable.
type SetADTEnvironmentVariableOptions struct {
	// Variable is the environment variable name.
	Variable string
	// Value is the value to assign.
	Value string
	// Target is Process (default), User or Machine.
	Target string
	// Expandable stores the value as REG_EXPAND_SZ so "%Var%" references
	// expand at query time (User/Machine targets only).
	Expandable bool
}

// SetADTEnvironmentVariable is the Go port of Set-ADTEnvironmentVariable: it
// sets an environment variable in the process block or, on Windows, in the
// User/Machine registry stores, broadcasting WM_SETTINGCHANGE afterwards.
func SetADTEnvironmentVariable(ctx context.Context, opts SetADTEnvironmentVariableOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if strings.TrimSpace(opts.Variable) == "" {
		return winerr.Wrap("SetADTEnvironmentVariable: a variable name is required",
			winerr.ErrInvalidOption)
	}
	target, err := parseEnvironmentTarget(opts.Target)
	if err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf("Setting the environment variable [%s] for [%s] to [%s].",
			opts.Variable, target, opts.Value),
		LogSeverityInfo, "Set-ADTEnvironmentVariable",
	)
	if target == EnvironmentTargetProcess {
		if err := os.Setenv(opts.Variable, opts.Value); err != nil {
			return fmt.Errorf("adt: setting environment variable %s: %w", opts.Variable, err)
		}
		return nil
	}
	return setEnvironmentVariableInRegistry(opts.Variable, opts.Value, target, opts.Expandable)
}

// RemoveADTEnvironmentVariableOptions mirrors Remove-ADTEnvironmentVariable.
type RemoveADTEnvironmentVariableOptions struct {
	// Variable is the environment variable name.
	Variable string
	// Target is Process (default), User or Machine.
	Target string
}

// RemoveADTEnvironmentVariable is the Go port of
// Remove-ADTEnvironmentVariable: it removes an environment variable from the
// process block or, on Windows, from the User/Machine registry stores,
// broadcasting WM_SETTINGCHANGE afterwards. Removing a variable that does
// not exist is not an error.
func RemoveADTEnvironmentVariable(
	ctx context.Context,
	opts RemoveADTEnvironmentVariableOptions,
) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if strings.TrimSpace(opts.Variable) == "" {
		return winerr.Wrap("RemoveADTEnvironmentVariable: a variable name is required",
			winerr.ErrInvalidOption)
	}
	target, err := parseEnvironmentTarget(opts.Target)
	if err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf("Removing the environment variable [%s] for [%s].", opts.Variable, target),
		LogSeverityInfo, "Remove-ADTEnvironmentVariable",
	)
	if target == EnvironmentTargetProcess {
		if err := os.Unsetenv(opts.Variable); err != nil {
			return fmt.Errorf("adt: removing environment variable %s: %w", opts.Variable, err)
		}
		return nil
	}
	return removeEnvironmentVariableFromRegistry(opts.Variable, target)
}
