package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

func newRunCommand() *cobra.Command {
	var (
		deploymentType string
		deployMode     string
		suppressReboot bool
	)
	cmd := &cobra.Command{
		Use:   "run [package-directory]",
		Short: "Run a deployment package (zero-config MSI when Files/ holds one MSI)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return fmt.Errorf("resolving package directory: %w", err)
			}
			return runPackage(cmd.Context(), abs, deploymentType, deployMode, suppressReboot)
		},
	}
	cmd.Flags().StringVar(&deploymentType, "deployment-type", "", "Install (default), Uninstall or Repair")
	cmd.Flags().StringVar(&deployMode, "deploy-mode", "", "Auto (default), Interactive, NonInteractive or Silent")
	cmd.Flags().BoolVar(&suppressReboot, "suppress-reboot-passthru", false, "return 0 instead of a reboot exit code")
	return cmd
}

// runPackage builds a Deployment for the package directory. When Files/ holds
// exactly one usable MSI, it wires the zero-config install/uninstall/repair
// phases; otherwise it errors, directing the user to a compiled Go deployment.
func runPackage(ctx context.Context, dir, deploymentType, deployMode string, suppressReboot bool) error {
	filesDir := filepath.Join(dir, "Files")
	msi, err := discoverZeroConfigMSI(filesDir)
	if err != nil {
		return err
	}

	product, err := readMSIProduct(ctx, msi)
	if err != nil {
		return err
	}

	dep := &adt.Deployment{
		Session: adt.SessionOptions{
			AppVendor:         product.vendor,
			AppName:           product.name,
			AppVersion:        product.version,
			ScriptDirectory:   dir,
			ConfigOverlayPath: optionalPath(filepath.Join(dir, "Config", "config.yaml")),
			StringsOverlayPath: optionalPath(filepath.Join(dir, "Strings", "strings.yaml")),
		},
		Args: buildRunArgs(deploymentType, deployMode, suppressReboot),
		Install: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{Action: "Install", Path: msi})
			return err
		},
		Uninstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{Action: "Uninstall", Path: msi})
			return err
		},
		Repair: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{Action: "Repair", Path: msi})
			return err
		},
	}
	dep.Run(ctx)
	return nil // Deployment.Run exits the process itself
}

func buildRunArgs(deploymentType, deployMode string, suppressReboot bool) []string {
	args := []string{}
	if deploymentType != "" {
		args = append(args, "-DeploymentType", deploymentType)
	}
	if deployMode != "" {
		args = append(args, "-DeployMode", deployMode)
	}
	if suppressReboot {
		args = append(args, "-SuppressRebootPassThru")
	}
	return args
}

// discoverZeroConfigMSI mirrors DeploymentSession's Zero-Config discovery:
// the single MSI in filesDir, preferring one whose name is not architecture-
// suffixed (e.g. "app.msi" over "app.x64.msi").
func discoverZeroConfigMSI(filesDir string) (string, error) {
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return "", fmt.Errorf("reading package Files directory: %w", err)
	}
	var msis []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".msi") {
			msis = append(msis, e.Name())
		}
	}
	if len(msis) == 0 {
		return "", fmt.Errorf("no MSI found in %s: %w", filesDir, adt.ErrNotFound)
	}
	sort.Strings(msis)
	chosen := msis[0]
	for _, m := range msis {
		if !isArchSuffixed(m) {
			chosen = m
			break
		}
	}
	return filepath.Join(filesDir, chosen), nil
}

func isArchSuffixed(name string) bool {
	base := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	for _, arch := range []string{".x64", ".x86", ".arm64", ".amd64"} {
		if strings.HasSuffix(base, arch) {
			return true
		}
	}
	return false
}

type msiProduct struct {
	vendor, name, version string
}

// readMSIProduct reads ProductName/Manufacturer/ProductVersion from the MSI
// Property table (Windows only); on non-Windows or read failure it falls back
// to the file name so the CLI still cross-compiles and self-tests.
func readMSIProduct(ctx context.Context, msiPath string) (msiProduct, error) {
	fallback := msiProduct{name: strings.TrimSuffix(filepath.Base(msiPath), filepath.Ext(msiPath)), version: "1.0.0"}
	props, err := adt.GetADTMsiTableProperty(ctx, adt.GetADTMsiTablePropertyOptions{Path: msiPath})
	if err != nil {
		return fallback, nil //nolint:nilerr // metadata is best-effort; name-based fallback is fine
	}
	p := msiProduct{
		vendor:  props["Manufacturer"],
		name:    props["ProductName"],
		version: props["ProductVersion"],
	}
	if p.name == "" {
		p.name = fallback.name
	}
	if p.version == "" {
		p.version = fallback.version
	}
	return p, nil
}

func optionalPath(p string) string {
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
