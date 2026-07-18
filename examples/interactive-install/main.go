// Command interactive-install demonstrates an interactive deployment with the
// welcome/close-apps prompt, a progress dialog, an EXE install and a
// completion prompt — the Go analogue of a typical Invoke-AppDeployToolkit.ps1.
//
// Build with GOOS=windows and run on a Windows host:
//
//	interactive-install.exe -DeploymentType Install -DeployMode Interactive
package main

import (
	"context"
	"errors"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/psadt"
)

func main() {
	(&psadt.Deployment{
		Session: psadt.SessionOptions{
			AppVendor:  "VideoLAN",
			AppName:    "VLC media player",
			AppVersion: "3.0.23",
			AppArch:    "x64",
			AppProcessesToClose: []psadt.ProcessObject{
				{Name: "vlc", Description: "VLC media player"},
			},
			RequireAdmin: true,
		},

		PreInstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			result, err := psadt.ShowADTInstallationWelcome(ctx, psadt.ShowADTInstallationWelcomeOptions{
				CloseProcesses:          s.Options().AppProcessesToClose,
				AllowDefer:              true,
				DeferTimes:              3,
				CloseProcessesCountdown: 60,
				PersistPrompt:           true,
			})
			if err != nil {
				return err
			}
			if result.Deferred {
				// The runner maps ErrDeferred to the configured defer exit code.
				return psadt.ErrDeferred
			}
			return psadt.ShowADTInstallationProgress(ctx, psadt.ShowADTInstallationProgressOptions{
				StatusMessage: "Installing VLC media player...",
			})
		},

		Install: func(ctx context.Context, s *psadt.DeploymentSession) error {
			_, err := psadt.StartADTProcess(ctx, psadt.StartADTProcessOptions{
				FilePath:     "vlc-3.0.23-win64.exe",
				ArgumentList: "/L=1033 /S",
			})
			return err
		},

		PostInstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			if err := psadt.CloseADTInstallationProgress(ctx); err != nil {
				return err
			}
			_, err := psadt.ShowADTInstallationPrompt(ctx, psadt.ShowADTInstallationPromptOptions{
				Message:         "VLC media player installation complete.",
				ButtonRightText: "OK",
				Icon:            "Information",
				NoWait:          true,
			})
			return err
		},

		Uninstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			err := psadt.UninstallADTApplication(ctx, psadt.UninstallADTApplicationOptions{
				Name:      []string{"VLC media player"},
				NameMatch: "Exact",
			})
			if errors.Is(err, psadt.ErrNotFound) {
				return nil // already gone
			}
			return err
		},
	}).Run(context.Background())
}
