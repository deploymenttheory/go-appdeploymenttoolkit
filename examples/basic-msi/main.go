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
	"path/filepath"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

const (
	appName    = "Example App"
	markerKey  = `HKLM:\SOFTWARE\Contoso\ExampleApp`
	msiProduct = "ExampleApp.msi" // resolved against the package Files\ directory
)

func main() {
	(&adt.Deployment{
		Session: adt.SessionOptions{
			AppVendor:    "Contoso",
			AppName:      appName,
			AppVersion:   "1.0.0",
			AppArch:      "x64",
			RequireAdmin: true,
		},

		Install: func(ctx context.Context, s *adt.DeploymentSession) error {
			// Install the MSI (silent params come from config in silent mode).
			if _, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{
				Action: "Install",
				Path:   msiProduct,
			}); err != nil {
				return err
			}

			// Drop a support file into the install location.
			if err := adt.CopyADTFile(ctx, adt.CopyADTFileOptions{
				Path:        []string{filepath.Join(s.DirSupportFiles(), "settings.json")},
				Destination: `$env:ProgramData\Contoso\ExampleApp`,
			}); err != nil && !errors.Is(err, adt.ErrNotFound) {
				return err
			}

			// Record a deployment marker other tooling can detect.
			return adt.SetADTRegistryKey(ctx, adt.SetADTRegistryKeyOptions{
				Key:   markerKey,
				Name:  "Version",
				Value: s.Options().AppVersion,
			})
		},

		PostInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			return adt.WriteADTLogEntry(ctx, adt.LogEntryOptions{
				Message:  []string{appName + " installed successfully."},
				Severity: adt.LogSeveritySuccess,
			})
		},

		Uninstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			// Prefer an exact uninstall by display name; fall back cleanly if
			// the product is already gone.
			err := adt.UninstallADTApplication(ctx, adt.UninstallADTApplicationOptions{
				Name:      []string{appName},
				NameMatch: "Exact",
			})
			if err != nil && !errors.Is(err, adt.ErrNotFound) {
				return err
			}
			return adt.RemoveADTRegistryKey(ctx, adt.RemoveADTRegistryKeyOptions{Key: markerKey})
		},
	}).Run(context.Background())
}
