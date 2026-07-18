package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newNewCommand() *cobra.Command {
	var appName string
	cmd := &cobra.Command{
		Use:   "new [directory]",
		Short: "Scaffold a new deployment package (Go analogue of New-ADTTemplate)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			return scaffold(dir, appName)
		},
	}
	cmd.Flags().StringVar(&appName, "name", "MyApplication", "application name used in the generated deployment")
	return cmd
}

// scaffold writes the package skeleton PSADT authors expect (Files,
// SupportFiles, Config, Strings, Assets) plus a main.go deployment program
// importing the SDK.
func scaffold(dir, appName string) error {
	for _, sub := range []string{"Files", "SupportFiles", "Config", "Strings", "Assets"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", sub, err)
		}
	}
	mainPath := filepath.Join(dir, "main.go")
	if _, err := os.Stat(mainPath); err == nil {
		return fmt.Errorf("%s already exists: refusing to overwrite", mainPath)
	}
	program := strings.ReplaceAll(deploymentTemplate, "{{AppName}}", appName)
	if err := os.WriteFile(mainPath, []byte(program), 0o644); err != nil {
		return fmt.Errorf("writing main.go: %w", err)
	}

	modulePath := "deployment/" + sanitizeModuleName(appName)
	goMod := fmt.Sprintf("module %s\n\ngo 1.25.0\n\nrequire %s latest\n", modulePath, sdkModulePath)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		return fmt.Errorf("writing go.mod: %w", err)
	}

	fmt.Printf("Scaffolded deployment for %q in %s\n", appName, dir)
	fmt.Println("Next steps:")
	fmt.Println("  1. cd", dir, "&& go mod tidy")
	fmt.Println("  2. Drop your installer under Files/ and edit the Install phase in main.go")
	fmt.Println("  3. GOOS=windows go build -o Invoke-AppDeployToolkit.exe")
	return nil
}

const sdkModulePath = "github.com/deploymenttheory/go-appdeploymenttoolkit"

// sanitizeModuleName reduces an app name to a module-path-safe token.
func sanitizeModuleName(name string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteByte('-')
		}
	}
	if b.Len() == 0 {
		return "app"
	}
	return b.String()
}

const deploymentTemplate = `// Deployment program for {{AppName}} (built with go-appdeploymenttoolkit).
//
// Build:  GOOS=windows go build -o Invoke-AppDeployToolkit.exe
// Run:    Invoke-AppDeployToolkit.exe -DeploymentType Install -DeployMode Interactive
package main

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/psadt"
)

func main() {
	(&psadt.Deployment{
		Session: psadt.SessionOptions{
			AppVendor:  "",
			AppName:    "{{AppName}}",
			AppVersion: "1.0.0",
			AppArch:    "x64",
		},

		PreInstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			_, err := psadt.ShowADTInstallationWelcome(ctx, psadt.ShowADTInstallationWelcomeOptions{
				CloseProcesses: []psadt.ProcessObject{},
				AllowDefer:     true,
				DeferTimes:     3,
			})
			return err
		},

		Install: func(ctx context.Context, s *psadt.DeploymentSession) error {
			// Example: install an MSI dropped under Files/.
			// _, err := psadt.StartADTMsiProcess(ctx, psadt.StartADTMsiProcessOptions{
			//     Action: "Install", Path: "{{AppName}}.msi",
			// })
			return psadt.WriteADTLogEntry(ctx, psadt.LogEntryOptions{
				Message: []string{"Installing {{AppName}}..."},
			})
		},

		PostInstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			return nil
		},

		Uninstall: func(ctx context.Context, s *psadt.DeploymentSession) error {
			return psadt.WriteADTLogEntry(ctx, psadt.LogEntryOptions{
				Message: []string{"Uninstalling {{AppName}}..."},
			})
		},
	}).Run(context.Background())
}
`
