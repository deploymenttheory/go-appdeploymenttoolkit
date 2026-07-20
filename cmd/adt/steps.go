package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

func newStepsCommand() *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "steps",
		Short: "List the manifest step catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			steps := manifest.Steps()
			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(steps)
			}
			tw := tabwriter.NewWriter(out, 2, 4, 2, ' ', 0)
			fmt.Fprintln(tw, "STEP\tPLATFORMS\tSUMMARY")
			for _, s := range steps {
				platforms := make([]string, len(s.Platforms))
				for i, p := range s.Platforms {
					platforms[i] = string(p)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\n", s.Name, strings.Join(platforms, ","), s.Summary)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the full step specs as JSON")
	return cmd
}
