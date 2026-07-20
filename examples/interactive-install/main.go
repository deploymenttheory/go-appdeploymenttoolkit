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

	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
)

func main() {
	(&winadt.Deployment{
		Session: winadt.SessionOptions{
			AppVendor:  "VideoLAN",
			AppName:    "VLC media player",
			AppVersion: "3.0.23",
			AppArch:    "x64",
			AppProcessesToClose: []winadt.ProcessObject{
				{Name: "vlc", Description: "VLC media player"},
			},
			RequireAdmin: true,
		},

		PreInstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			result, err := winadt.ShowADTInstallationWelcome(ctx, winadt.ShowADTInstallationWelcomeOptions{
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
				return winadt.ErrDeferred
			}
			return winadt.ShowADTInstallationProgress(ctx, winadt.ShowADTInstallationProgressOptions{
				StatusMessage: "Installing VLC media player...",
			})
		},

		Install: func(ctx context.Context, s *winadt.DeploymentSession) error {
			_, err := winadt.StartADTProcess(ctx, winadt.StartADTProcessOptions{
				FilePath:     "vlc-3.0.23-win64.exe",
				ArgumentList: "/L=1033 /S",
			})
			return err
		},

		PostInstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			if err := winadt.CloseADTInstallationProgress(ctx); err != nil {
				return err
			}
			_, err := winadt.ShowADTInstallationPrompt(ctx, winadt.ShowADTInstallationPromptOptions{
				Message:         "VLC media player installation complete.",
				ButtonRightText: "OK",
				Icon:            "Information",
				NoWait:          true,
			})
			return err
		},

		Uninstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			err := winadt.UninstallADTApplication(ctx, winadt.UninstallADTApplicationOptions{
				Name:      []string{"VLC media player"},
				NameMatch: "Exact",
			})
			if errors.Is(err, winadt.ErrNotFound) {
				return nil // already gone
			}
			return err
		},
	}).Run(context.Background())
}
