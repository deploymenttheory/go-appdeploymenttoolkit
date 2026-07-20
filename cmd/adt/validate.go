package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

// manifestFileNames are the conventional manifest names looked up in a
// package directory.
var manifestFileNames = []string{"deployment.yaml", "deployment.yml"}

// resolveManifestArg turns the positional argument (package dir or manifest
// file, default ".") into a manifest path and its package directory.
func resolveManifestArg(arg string) (manifestPath, packageDir string, err error) {
	if arg == "" {
		arg = "."
	}
	abs, err := filepath.Abs(arg)
	if err != nil {
		return "", "", fmt.Errorf("resolving %s: %w", arg, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", "", fmt.Errorf("reading %s: %w", abs, err)
	}
	if !info.IsDir() {
		return abs, filepath.Dir(abs), nil
	}
	for _, name := range manifestFileNames {
		candidate := filepath.Join(abs, name)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, abs, nil
		}
	}
	return "", "", fmt.Errorf("no %s found in %s", strings.Join(manifestFileNames, " or "), abs)
}

func newValidateCommand() *cobra.Command {
	var (
		packageDir string
		target     string
		asJSON     bool
	)
	cmd := &cobra.Command{
		Use:   "validate [package-dir | manifest.yaml]",
		Short: "Validate a deployment manifest (schema, semantics, files, platform)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			arg := ""
			if len(args) == 1 {
				arg = args[0]
			}
			manifestPath, derivedPkg, err := resolveManifestArg(arg)
			if err != nil {
				return err
			}
			if packageDir == "" {
				packageDir = derivedPkg
			}
			tgt := manifest.Platform(strings.ToLower(target))
			if tgt != manifest.PlatformWindows && tgt != manifest.PlatformDarwin {
				return fmt.Errorf("unsupported --target %q (windows or darwin)", target)
			}

			m, issues, err := manifest.Load(manifestPath)
			if err != nil {
				return err
			}
			issues = append(issues, manifest.Validate(m, manifest.ValidateOptions{
				PackageDir: packageDir,
				Target:     tgt,
			})...)
			report := manifest.NewReport(manifestPath, issues)

			out := cmd.OutOrStdout()
			if asJSON {
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				if err := enc.Encode(report); err != nil {
					return err
				}
			} else {
				printHumanReport(out, report)
			}
			if !report.Valid {
				return fmt.Errorf("%s failed validation", filepath.Base(manifestPath))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&packageDir, "package-dir", "", "package root for file-existence checks (default: the manifest's directory)")
	cmd.Flags().StringVar(&target, "target", "windows", "target platform steps must support (windows or darwin)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "emit the machine-readable report")
	return cmd
}

// printHumanReport writes gcc-style issue lines and a summary.
func printHumanReport(out interface{ Write([]byte) (int, error) }, r manifest.Report) {
	name := filepath.Base(r.Manifest)
	errs, warns := 0, 0
	for _, i := range r.Issues {
		if i.Severity == manifest.SeverityError {
			errs++
		} else {
			warns++
		}
		fmt.Fprintf(out, "%s:%d:%d: %s [%s] %s: %s\n",
			name, i.Pos.Line, i.Pos.Col, i.Severity, i.Code, i.Path, i.Message)
	}
	switch {
	case errs == 0 && warns == 0:
		fmt.Fprintf(out, "%s: valid\n", name)
	default:
		fmt.Fprintf(out, "%d error(s), %d warning(s)\n", errs, warns)
	}
}
