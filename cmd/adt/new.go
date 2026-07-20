package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed templates/config.yaml
var sampleConfig []byte

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

	// Seed a commented config overlay so authors can see and tune the knobs.
	configPath := filepath.Join(dir, "Config", "config.yaml")
	if _, err := os.Stat(configPath); err != nil {
		if err := os.WriteFile(configPath, sampleConfig, 0o644); err != nil {
			return fmt.Errorf("writing Config/config.yaml: %w", err)
		}
	}

	// Seed a starter manifest: the no-code path (`adt validate` + `adt run`).
	manifestPath := filepath.Join(dir, "deployment.yaml")
	if _, err := os.Stat(manifestPath); err != nil {
		starter := strings.ReplaceAll(manifestTemplate, "{{AppName}}", appName)
		if err := os.WriteFile(manifestPath, []byte(starter), 0o644); err != nil {
			return fmt.Errorf("writing deployment.yaml: %w", err)
		}
	}

	fmt.Printf("Scaffolded deployment for %q in %s\n", appName, dir)
	fmt.Println("Next steps (manifest workflow):")
	fmt.Println("  1. Drop your installer under Files/ and edit deployment.yaml (`adt steps` lists the step catalog)")
	fmt.Println("  2. adt validate", dir)
	fmt.Println("  3. adt run", dir)
	fmt.Println("Or the compiled-Go workflow:")
	fmt.Println("  1. cd", dir, "&& go mod tidy, then edit the phases in main.go")
	fmt.Println("  2. GOOS=windows go build -o Invoke-AppDeployToolkit.exe")
	fmt.Println("(adt run prefers deployment.yaml when present; delete it if you only want main.go)")
	return nil
}

// manifestTemplate is the starter deployment.yaml emitted by `adt new`.
const manifestTemplate = `# Deployment manifest for {{AppName}}.
# Validate with:  adt validate .
# Run with:       adt run . --deploy-mode Silent
# Step catalog:   adt steps
apiVersion: v0.1.0-alpha
kind: Deployment

session:
  appVendor: ""
  appName: {{AppName}}
  appVersion: "1.0.0"
  appArch: x64
  # Machine-wide installs need elevation; fail fast without admin rights.
  requireAdmin: true
  # Apps the welcome dialog offers to close (also drives Auto deploy-mode
  # detection: with none listed, Auto resolves Silent).
  closeProcesses: []

phases:
  preInstall:
    - uses: dialog.welcome
      with:
        allowDefer: true
        deferTimes: 3
  install:
    # Replace with your installer, e.g.:
    # - uses: msi.install
    #   with: {path: {{AppName}}.msi}
    - uses: dialog.progress
      with: {statusMessage: "Installing {{AppName}}..."}
  postInstall:
    - uses: dialog.progressClose
  uninstall:
    # - uses: msi.uninstall
    #   with: {path: "{product-code-guid}"}
    []
`

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

	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
)

func main() {
	(&winadt.Deployment{
		Session: winadt.SessionOptions{
			AppVendor:  "",
			AppName:    "{{AppName}}",
			AppVersion: "1.0.0",
			AppArch:    "x64",
			// Machine-wide MSI installs require elevation; this fails fast with
			// a clear message when run without admin rights.
			RequireAdmin: true,
		},

		PreInstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			_, err := winadt.ShowADTInstallationWelcome(ctx, winadt.ShowADTInstallationWelcomeOptions{
				CloseProcesses: []winadt.ProcessObject{},
				AllowDefer:     true,
				DeferTimes:     3,
			})
			return err
		},

		Install: func(ctx context.Context, s *winadt.DeploymentSession) error {
			// Example: install an MSI dropped under Files/.
			// _, err := winadt.StartADTMsiProcess(ctx, winadt.StartADTMsiProcessOptions{
			//     Action: "Install", Path: "{{AppName}}.msi",
			// })
			return winadt.WriteADTLogEntry(ctx, winadt.LogEntryOptions{
				Message: []string{"Installing {{AppName}}..."},
			})
		},

		PostInstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			return nil
		},

		Uninstall: func(ctx context.Context, s *winadt.DeploymentSession) error {
			return winadt.WriteADTLogEntry(ctx, winadt.LogEntryOptions{
				Message: []string{"Uninstalling {{AppName}}..."},
			})
		},
	}).Run(context.Background())
}
`
