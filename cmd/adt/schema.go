package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

func newSchemaCommand() *cobra.Command {
	var target string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the deployment-manifest JSON Schema (v" + manifest.SchemaVersion + ")",
		Long: "Print the JSON Schema (draft 2020-12) generated from the step registry\n" +
			"for the selected --target platform. Emit it beside a package for editor\n" +
			"autocomplete and inline validation:\n\n" +
			"  adt schema > " + manifest.SchemaFileName + "\n\n" +
			"then reference it from deployment.yaml:\n\n" +
			"  # yaml-language-server: $schema=./" + manifest.SchemaFileName,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			tgt := manifest.Platform(strings.ToLower(target))
			if tgt != manifest.PlatformWindows && tgt != manifest.PlatformDarwin {
				return fmt.Errorf("unsupported --target %q (windows or darwin)", target)
			}
			blob, err := manifest.JSONSchema(tgt)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(blob))
			return err
		},
	}
	cmd.Flags().StringVar(&target, "target", "windows", "platform whose step catalog the schema describes (windows or darwin)")
	return cmd
}
