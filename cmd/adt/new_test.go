package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/config"
)

// TestSampleConfigLoads ensures the scaffolded Config/config.yaml parses
// through the real loader, overrides what it sets, and leaves other defaults
// intact — so `adt new` never emits a config the toolkit can't read.
func TestSampleConfigLoads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, sampleConfig, 0o644))

	c, err := config.Load(path)
	require.NoError(t, err)

	// Values the sample sets explicitly.
	assert.Equal(t, "CMTrace", c.Toolkit.LogStyle)
	assert.Equal(t, "Native", c.Toolkit.FileCopyMode)
	assert.Equal(t, "Fluent", c.UI.DialogStyle)
	assert.Equal(t, 1602, c.UI.DeferExitCode)
	assert.Equal(t, 600, c.MSI.MutexWaitTime)
	// A default the sample does not mention still resolves.
	assert.Equal(t, 3300, c.UI.DefaultTimeout)
}

func TestScaffoldWritesPackage(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, scaffold(dir, "Demo App"))

	for _, f := range []string{"main.go", "go.mod", filepath.Join("Config", "config.yaml")} {
		_, err := os.Stat(filepath.Join(dir, f))
		assert.NoError(t, err, f)
	}
	// The generated main.go references the SDK by its adt import.
	program, err := os.ReadFile(filepath.Join(dir, "main.go"))
	require.NoError(t, err)
	assert.Contains(t, string(program), "go-appdeploymenttoolkit/adt")
	assert.Contains(t, string(program), "Demo App")
	// The scaffold defaults to requiring admin so a non-elevated run fails fast.
	assert.Contains(t, string(program), "RequireAdmin: true")
}
