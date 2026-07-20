// Command basic-msi is a complete, non-interactive MSI deployment — the Go
// analogue of a silent Invoke-AppDeployToolkit.ps1. It installs an MSI from
// the package's Files\ directory, copies a support file, writes a deployment
// marker to the registry, and cleans all of that up on uninstall.
//
// Build for Windows and run on a Windows host:
//
//	GOOS=windows go build -o Invoke-AppDeployToolkit.exe
//	Invoke-AppDeployToolkit.exe -DeploymentType Install  -DeployMode Silent
//	Invoke-AppDeployToolkit.exe -DeploymentType Uninstall -DeployMode Silent
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
)

const (
	appName    = "Example App"
	markerKey  = `HKLM:\SOFTWARE\Contoso\ExampleApp`
	msiProduct = "ExampleApp.msi" // resolved against the package Files\ directory
)

func main() {
	(&winadt.Deployment{
		Session: winadt.SessionOptions{
			AppVendor:    "Contoso",
			AppName:      appName,
			AppVersion:   "1.0.0",
			AppArch:      "x64",
			RequireAdmin: true,
		},

		Install: func(ctx context.Context, s *winadt.DeploymentSession) error {
			// Install the MSI (silent params come from config in silent mode).
			if _, err := winadt.StartADTMsiProcess(ctx, winadt.StartADTMsiProcessOptions{
				Action: "Install",
				Path:   msiProduct,
			}); err != nil {
				return err
			}

			// Drop a support file into the install location. CopyADTFile takes
			// concrete paths — unlike config values it does not expand $env:
			// tokens (matching PSADT, where the shell expands them first), so
			// resolve environment variables in Go.
			if err := winadt.CopyADTFile(ctx, winadt.CopyADTFileOptions{
				Path:        []string{filepath.Join(s.DirSupportFiles(), "settings.json")},
				Destination: filepath.Join(os.Getenv("ProgramData"), "Contoso", "ExampleApp"),
			}); err != nil && !errors.Is(err, winadt.ErrNotFound) {
				return err
			}

			// Record a deployment marker other tooling can detect.
			return winadt.SetADTRegistryKey(ctx, winadt.SetADTRegistryKeyOptions{
				Key:   markerKey,
				Name:  "Version",
				Value: s.Options().AppVersion,
			})
		},

		PostInstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			return winadt.WriteADTLogEntry(ctx, winadt.LogEntryOptions{
				Message:  []string{appName + " installed successfully."},
				Severity: winadt.LogSeveritySuccess,
			})
		},

		Uninstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			// Prefer an exact uninstall by display name; fall back cleanly if
			// the product is already gone.
			err := winadt.UninstallADTApplication(ctx, winadt.UninstallADTApplicationOptions{
				Name:      []string{appName},
				NameMatch: "Exact",
			})
			if err != nil && !errors.Is(err, winadt.ErrNotFound) {
				return err
			}
			return winadt.RemoveADTRegistryKey(ctx, winadt.RemoveADTRegistryKeyOptions{Key: markerKey})
		},
	}).Run(context.Background())
}
