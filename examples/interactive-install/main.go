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

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

func main() {
	(&adt.Deployment{
		Session: adt.SessionOptions{
			AppVendor:  "VideoLAN",
			AppName:    "VLC media player",
			AppVersion: "3.0.23",
			AppArch:    "x64",
			AppProcessesToClose: []adt.ProcessObject{
				{Name: "vlc", Description: "VLC media player"},
			},
			RequireAdmin: true,
		},

		PreInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			result, err := adt.ShowADTInstallationWelcome(ctx, adt.ShowADTInstallationWelcomeOptions{
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
				return adt.ErrDeferred
			}
			return adt.ShowADTInstallationProgress(ctx, adt.ShowADTInstallationProgressOptions{
				StatusMessage: "Installing VLC media player...",
			})
		},

		Install: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTProcess(ctx, adt.StartADTProcessOptions{
				FilePath:     "vlc-3.0.23-win64.exe",
				ArgumentList: "/L=1033 /S",
			})
			return err
		},

		PostInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			if err := adt.CloseADTInstallationProgress(ctx); err != nil {
				return err
			}
			_, err := adt.ShowADTInstallationPrompt(ctx, adt.ShowADTInstallationPromptOptions{
				Message:         "VLC media player installation complete.",
				ButtonRightText: "OK",
				Icon:            "Information",
				NoWait:          true,
			})
			return err
		},

		Uninstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			err := adt.UninstallADTApplication(ctx, adt.UninstallADTApplicationOptions{
				Name:      []string{"VLC media player"},
				NameMatch: "Exact",
			})
			if errors.Is(err, adt.ErrNotFound) {
				return nil // already gone
			}
			return err
		},
	}).Run(context.Background())
}
