package psadt

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shortcut"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Shortcut models the properties of a .lnk or .url shortcut, mirroring the
// parameters of PSADT's *-ADTShortcut functions.
type Shortcut = shortcut.Shortcut

// ShortcutWindowStyle mirrors the WindowStyle parameter values.
type ShortcutWindowStyle = shortcut.WindowStyle

// ShortcutWindowStyle values.
const (
	ShortcutWindowStyleNormal    = shortcut.WindowStyleNormal
	ShortcutWindowStyleMaximized = shortcut.WindowStyleMaximized
	ShortcutWindowStyleMinimized = shortcut.WindowStyleMinimized
)

// NewADTShortcut is the Go port of New-ADTShortcut: it creates a .lnk shell
// link (Windows) or .url internet shortcut at s.Path.
func NewADTShortcut(ctx context.Context, s Shortcut) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return fmt.Errorf("psadt: creating shortcut directory: %w", err)
	}
	logSessionInfo(fmt.Sprintf("Creating shortcut [%s].", s.Path), "NewADTShortcut")
	return shortcut.Create(&s)
}

// GetADTShortcut is the Go port of Get-ADTShortcut: it reads the properties
// of an existing shortcut.
func GetADTShortcut(ctx context.Context, path string) (*Shortcut, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("psadt: %w", err)
	}
	return shortcut.Read(path)
}

// SetADTShortcutOptions carries the mutable properties for SetADTShortcut;
// nil pointer fields are left unchanged, mirroring Set-ADTShortcut's
// parameter behavior.
type SetADTShortcutOptions struct {
	TargetPath       *string
	Arguments        *string
	Description      *string
	WorkingDirectory *string
	IconLocation     *string
	IconIndex        *int
	WindowStyle      *ShortcutWindowStyle
	Hotkey           *string
}

// SetADTShortcut is the Go port of Set-ADTShortcut: it updates the provided
// properties of an existing shortcut, leaving the rest intact.
func SetADTShortcut(ctx context.Context, path string, opts SetADTShortcutOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: %w", err)
	}
	logSessionInfo(fmt.Sprintf("Updating shortcut [%s].", path), "SetADTShortcut")
	return shortcut.Update(path, func(current *Shortcut) {
		if opts.TargetPath != nil {
			current.TargetPath = *opts.TargetPath
		}
		if opts.Arguments != nil {
			current.Arguments = *opts.Arguments
		}
		if opts.Description != nil {
			current.Description = *opts.Description
		}
		if opts.WorkingDirectory != nil {
			current.WorkingDirectory = *opts.WorkingDirectory
		}
		if opts.IconLocation != nil {
			current.IconLocation = *opts.IconLocation
		}
		if opts.IconIndex != nil {
			current.IconIndex = *opts.IconIndex
		}
		if opts.WindowStyle != nil {
			current.WindowStyle = *opts.WindowStyle
		}
		if opts.Hotkey != nil {
			current.Hotkey = *opts.Hotkey
		}
	})
}

// RemoveADTDesktopShortcut is the Go port of Remove-ADTDesktopShortcut: it
// removes "<Name>.lnk" (or the exact name when an extension is included)
// from the common desktop, ignoring shortcuts that do not exist.
func RemoveADTDesktopShortcut(ctx context.Context, name string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: %w", err)
	}
	if filepath.Ext(name) == "" {
		name += ".lnk"
	}
	desktop := os.Getenv("PUBLIC")
	if desktop == "" {
		if env, err := GetADTEnvironmentTable(); err == nil && env.EnvCommonDesktop != "" {
			desktop = filepath.Dir(env.EnvCommonDesktop)
		}
	}
	if desktop == "" {
		return winerr.Wrap("psadt: common desktop path unresolved", winerr.ErrNotFound)
	}
	target := filepath.Join(desktop, "Desktop", name)
	err := os.Remove(target)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("psadt: removing desktop shortcut: %w", err)
	}
	logSessionInfo(fmt.Sprintf("Removed desktop shortcut [%s].", target), "RemoveADTDesktopShortcut")
	return nil
}
