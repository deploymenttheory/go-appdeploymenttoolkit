package main

import (
	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogclient"
)

// newClientCommand exposes the hidden dialog-client entrypoint. The dialog
// server re-execs this same binary as `adt client ...` inside the interactive
// user session to render UI when the deployment itself runs as SYSTEM.
func newClientCommand() *cobra.Command {
	var (
		pipeIn  string
		pipeOut string
		nonce   string
	)
	cmd := &cobra.Command{
		Use:    "client",
		Short:  "Internal: render deployment dialogs in the user session",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return dialogclient.ClientMain(cmd.Context(), dialogclient.Config{
				PipeIn:  pipeIn,
				PipeOut: pipeOut,
				Nonce:   nonce,
			})
		},
	}
	cmd.Flags().StringVar(&pipeIn, "pipe-in", "", "inherited read-pipe handle")
	cmd.Flags().StringVar(&pipeOut, "pipe-out", "", "inherited write-pipe handle")
	cmd.Flags().StringVar(&nonce, "nonce", "", "handshake nonce")
	return cmd
}
