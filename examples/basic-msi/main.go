// Command basic-msi is the Go analogue of a minimal Invoke-AppDeployToolkit.ps1:
// a silent-capable MSI deployment with welcome prompt and logging.
//
// Build with GOOS=windows and run on a Windows host:
//
//	basic-msi.exe -DeploymentType Install -DeployMode Silent
package main

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

func main() {
	(&adt.Deployment{
		Session: adt.SessionOptions{
			AppVendor:           "Contoso",
			AppName:             "Example App",
			AppVersion:          "1.0.0",
			AppArch:             "x64",
			AppProcessesToClose: []adt.ProcessObject{{Name: "notepad", Description: "Notepad"}},
			RequireAdmin:        false,
		},

		PreInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			return adt.WriteADTLogEntry(ctx, adt.LogEntryOptions{
				Message: []string{"Preparing installation of " + s.InstallTitle()},
			})
		},

		Install: func(ctx context.Context, s *adt.DeploymentSession) error {
			// Phase 2 will provide StartADTMsiProcess; until then this logs intent.
			return adt.WriteADTLogEntry(ctx, adt.LogEntryOptions{
				Message: []string{"Would install " + s.InstallTitle() + " from " + s.DirFiles()},
			})
		},

		PostInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			return adt.WriteADTLogEntry(ctx, adt.LogEntryOptions{
				Message:  []string{"Installation finished."},
				Severity: adt.LogSeveritySuccess,
			})
		},
	}).Run(context.Background())
}
