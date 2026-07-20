package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

func newSchemaCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "schema",
		Short: "Print the deployment-manifest JSON Schema (v" + manifest.SchemaVersion + ")",
		Long: "Print the JSON Schema (draft 2020-12) generated from the step registry.\n" +
			"Emit it beside a package for editor autocomplete and inline validation:\n\n" +
			"  adt schema > " + manifest.SchemaFileName + "\n\n" +
			"then reference it from deployment.yaml:\n\n" +
			"  # yaml-language-server: $schema=./" + manifest.SchemaFileName,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			blob, err := manifest.JSONSchema()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(blob))
			return err
		},
	}
}
