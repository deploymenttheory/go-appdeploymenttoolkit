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
	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

func newRunCommand() *cobra.Command {
	var (
		deploymentType string
		deployMode     string
		suppressReboot bool
		detection      detectionFlags
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
			return runPackage(cmd.Context(), abs, deploymentType, deployMode, suppressReboot, detection)
		},
	}
	cmd.Flags().StringVar(&deploymentType, "deployment-type", "", "Install (default), Uninstall or Repair")
	cmd.Flags().StringVar(&deployMode, "deploy-mode", "", "Auto (default), Interactive, NonInteractive or Silent")
	cmd.Flags().BoolVar(&suppressReboot, "suppress-reboot-passthru", false, "return 0 instead of a reboot exit code")
	cmd.Flags().BoolVar(&detection.noOobe, "no-oobe-detection", false, "skip OOBE/ESP checks during Auto deploy-mode resolution")
	cmd.Flags().BoolVar(&detection.noProcess, "no-process-detection", false, "skip processes-to-close checks during Auto deploy-mode resolution")
	cmd.Flags().BoolVar(&detection.noSession, "no-session-detection", false, "skip session-0 checks during Auto deploy-mode resolution")
	cmd.Flags().BoolVar(&detection.interactivity, "process-interactivity-detection", false, "in session 0, require an interactive window station")
	cmd.Flags().BoolVar(&detection.noMsiProcessList, "no-default-msi-process-list", false, "do not derive processes-to-close from the MSI File table")
	return cmd
}

// detectionFlags carries the Auto deploy-mode detection opt-outs.
type detectionFlags struct {
	noOobe, noProcess, noSession, interactivity bool
	noMsiProcessList                            bool
}

// runPackage builds a Deployment for the package directory. A deployment
// manifest (deployment.yaml) takes precedence; otherwise, when Files/ holds
// exactly one usable MSI (or a WIM containing one), the zero-config
// install/uninstall/repair phases are wired; otherwise it errors, directing
// the user to a manifest or a compiled Go deployment.
func runPackage(ctx context.Context, dir, deploymentType, deployMode string, suppressReboot bool, detection detectionFlags) error {
	if manifestPath, pkgDir, err := resolveManifestArg(dir); err == nil {
		return runManifest(ctx, manifestPath, pkgDir, deploymentType, deployMode, suppressReboot, detection)
	}
	filesDir := filepath.Join(dir, "Files")
	msi, err := discoverZeroConfigMSI(filesDir)
	var wimMount string
	if err != nil {
		// Zero-Config WIM fallback: mount a WIM found in Files/ and look for
		// the MSI inside it (PSADT's ForceWimDetection flow).
		msi, wimMount, err = discoverZeroConfigWIM(ctx, filesDir)
		if err != nil {
			return err
		}
		defer func() {
			_ = adt.DismountADTWimFile(ctx, adt.DismountADTWimFileOptions{Path: wimMount})
		}()
	}
	transforms := discoverTransforms(msi)
	patches := discoverPatches(filesDir)

	product, err := readMSIProduct(ctx, msi)
	if err != nil {
		return err
	}

	// Processes-to-close from the MSI File table's .exe entries, like
	// PSADT's Zero-Config discovery (opt out with
	// --no-default-msi-process-list). Best-effort: discovery failures leave
	// the list empty.
	var processesToClose []adt.ProcessObject
	if !detection.noMsiProcessList {
		if procs, err := adt.GetADTMsiExeProcessList(ctx, msi, transforms); err == nil {
			processesToClose = procs
		}
	}

	msiOpts := func(action string) adt.StartADTMsiProcessOptions {
		return adt.StartADTMsiProcessOptions{Action: action, Path: msi, Transforms: transforms}
	}
	dep := &adt.Deployment{
		Session: adt.SessionOptions{
			AppVendor:           product.vendor,
			AppName:             product.name,
			AppVersion:          product.version,
			ScriptDirectory:     dir,
			ConfigOverlayPath:   optionalPath(filepath.Join(dir, "Config", "config.yaml")),
			StringsOverlayPath:  optionalPath(filepath.Join(dir, "Strings", "strings.yaml")),
			AppProcessesToClose: processesToClose,
			// Zero-config actions are MSI install/uninstall/repair, all of which
			// need elevation; fail fast with a clear message otherwise.
			RequireAdmin: true,

			NoOobeDetection:               detection.noOobe,
			NoProcessDetection:            detection.noProcess,
			NoSessionDetection:            detection.noSession,
			ProcessInteractivityDetection: detection.interactivity,
		},
		Args: buildRunArgs(deploymentType, deployMode, suppressReboot),
		Install: func(ctx context.Context, s *adt.DeploymentSession) error {
			if _, err := adt.StartADTMsiProcess(ctx, msiOpts("Install")); err != nil {
				return err
			}
			// Patches apply after the base install, alphabetical = order.
			for _, msp := range patches {
				if _, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{
					Action: "Patch",
					Path:   msp,
				}); err != nil {
					return err
				}
			}
			return nil
		},
		Uninstall: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, msiOpts("Uninstall"))
			return err
		},
		Repair: func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, msiOpts("Repair"))
			return err
		},
	}
	dep.Run(ctx)
	return nil // Deployment.Run exits the process itself
}

// discoverZeroConfigWIM mounts the first .wim in filesDir to a mount folder
// beneath it and discovers the zero-config MSI inside the image. Returns the
// MSI path and the mount point (the caller dismounts).
func discoverZeroConfigWIM(ctx context.Context, filesDir string) (msi, mount string, err error) {
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return "", "", fmt.Errorf("reading package Files directory: %w", err)
	}
	var wims []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".wim") {
			wims = append(wims, filepath.Join(filesDir, e.Name()))
		}
	}
	if len(wims) == 0 {
		return "", "", fmt.Errorf("no MSI or WIM found in %s: %w", filesDir, adt.ErrNotFound)
	}
	sort.Strings(wims)
	mount = filepath.Join(filesDir, "wim-mount")
	if _, err := adt.MountADTWimFile(ctx, adt.MountADTWimFileOptions{
		ImagePath: wims[0],
		Path:      mount,
		Index:     1,
		Force:     true,
	}); err != nil {
		return "", "", fmt.Errorf("mounting %s: %w", wims[0], err)
	}
	msi, err = discoverZeroConfigMSI(mount)
	if err != nil {
		_ = adt.DismountADTWimFile(ctx, adt.DismountADTWimFileOptions{Path: mount})
		return "", "", err
	}
	return msi, mount, nil
}

// discoverTransforms returns the MST beside the MSI sharing its base name
// (app.msi -> app.mst), PSADT's Zero-Config transform convention.
func discoverTransforms(msiPath string) []string {
	mst := strings.TrimSuffix(msiPath, filepath.Ext(msiPath)) + ".mst"
	if _, err := os.Stat(mst); err != nil {
		return nil
	}
	return []string{mst}
}

// discoverPatches returns every .msp in the Files directory in alphabetical
// order (= application order), PSADT's Zero-Config patch convention.
func discoverPatches(filesDir string) []string {
	entries, err := os.ReadDir(filesDir)
	if err != nil {
		return nil
	}
	var patches []string
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(filepath.Ext(e.Name()), ".msp") {
			patches = append(patches, filepath.Join(filesDir, e.Name()))
		}
	}
	sort.Strings(patches)
	return patches
}

// runManifest loads, validates and compiles a deployment manifest, then runs
// it with the CLI flags overriding the manifest's session intent.
func runManifest(
	ctx context.Context,
	manifestPath, pkgDir, deploymentType, deployMode string,
	suppressReboot bool,
	detection detectionFlags,
) error {
	dep, issues, err := manifest.LoadAndCompile(manifestPath, manifest.CompileOptions{PackageDir: pkgDir})
	if err != nil {
		printHumanReport(os.Stderr, manifest.NewReport(manifestPath, issues))
		return err
	}
	if detection.noOobe {
		dep.Session.NoOobeDetection = true
	}
	if detection.noProcess {
		dep.Session.NoProcessDetection = true
	}
	if detection.noSession {
		dep.Session.NoSessionDetection = true
	}
	if detection.interactivity {
		dep.Session.ProcessInteractivityDetection = true
	}
	dep.Args = buildRunArgs(deploymentType, deployMode, suppressReboot)
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
