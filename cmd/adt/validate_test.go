package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/manifest"
)

// writePackage lays out a package dir with the given manifest body and Files/.
func writePackage(t *testing.T, manifestBody string, files ...string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "deployment.yaml"), []byte(manifestBody), 0o644))
	for _, f := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "Files", f), []byte("x"), 0o644))
	}
	return dir
}

const validManifest = `apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: CLI Test}
phases:
  install:
    - uses: msi.install
      with: {path: App.msi}
`

const invalidManifest = `apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: CLI Test}
phases:
  install:
    - uses: msi.instal
`

// runCLI executes a cobra command capturing stdout.
func runCLI(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	// Mirror the production root command's silencing (main.go prints errors).
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return buf.String(), err
}

func TestValidateCommandValid(t *testing.T) {
	pkg := writePackage(t, validManifest, "App.msi")
	out, err := runCLI(t, newValidateCommand(), pkg)
	require.NoError(t, err)
	assert.Contains(t, out, "valid")
}

func TestValidateCommandInvalid(t *testing.T) {
	pkg := writePackage(t, invalidManifest)
	out, err := runCLI(t, newValidateCommand(), pkg)
	require.Error(t, err)
	assert.Contains(t, out, "deployment.yaml:6:")
	assert.Contains(t, out, "unknown-step")
	assert.Contains(t, out, "error(s)")
}

func TestValidateCommandMissingFile(t *testing.T) {
	pkg := writePackage(t, validManifest) // App.msi absent
	out, err := runCLI(t, newValidateCommand(), pkg)
	require.Error(t, err)
	assert.Contains(t, out, "missing-file")
}

func TestValidateCommandJSON(t *testing.T) {
	pkg := writePackage(t, invalidManifest)
	out, err := runCLI(t, newValidateCommand(), pkg, "--json")
	require.Error(t, err)
	var report manifest.Report
	require.NoError(t, json.Unmarshal([]byte(out), &report))
	assert.False(t, report.Valid)
	require.NotEmpty(t, report.Issues)
	assert.Equal(t, "unknown-step", report.Issues[0].Code)
}

func TestValidateCommandTargetDarwin(t *testing.T) {
	pkg := writePackage(t, validManifest, "App.msi")
	out, err := runCLI(t, newValidateCommand(), pkg, "--target", "darwin")
	require.Error(t, err)
	assert.Contains(t, out, "platform-unsupported")
}

func TestStepsCommand(t *testing.T) {
	out, err := runCLI(t, newStepsCommand())
	require.NoError(t, err)
	assert.Contains(t, out, "msi.install")
	assert.Contains(t, out, "dialog.welcome")
	assert.Contains(t, out, "windows")
}

func TestStepsCommandJSON(t *testing.T) {
	out, err := runCLI(t, newStepsCommand(), "--json")
	require.NoError(t, err)
	var steps []manifest.StepSpec
	require.NoError(t, json.Unmarshal([]byte(out), &steps))
	names := map[string]bool{}
	for _, s := range steps {
		names[s.Name] = true
	}
	assert.True(t, names["msi.install"])
	assert.True(t, names["registry.set"])
}

func TestScaffoldedManifestValidates(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, scaffold(dir, "ScaffoldTest"))
	m, issues, err := manifest.Load(filepath.Join(dir, "deployment.yaml"))
	require.NoError(t, err)
	require.Empty(t, issues, "scaffolded manifest must be schema-clean: %+v", issues)
	issues = manifest.Validate(m, manifest.ValidateOptions{PackageDir: dir})
	for _, i := range issues {
		assert.NotEqual(t, manifest.SeverityError, i.Severity, "scaffold issue: %+v", i)
	}
}
