// Command adt is the Go analogue of Invoke-AppDeployToolkit.exe: it runs a
// deployment package from the command line. The primary use is the
// zero-config MSI flow (`adt run <package-dir>`), which discovers a single
// MSI under Files/ and drives install/uninstall/repair with no author code,
// mirroring PSADT's Zero-Config deployment.
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:           "adt",
		Short:         "Go App Deploy Toolkit runner (PSAppDeployToolkit port)",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newRunCommand(), newNewCommand(), newClientCommand(), newValidateCommand(), newStepsCommand(), newSchemaCommand())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "adt:", err)
		os.Exit(1)
	}
}
