package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultMatchesPSADTValues(t *testing.T) {
	c, err := Default()
	require.NoError(t, err)

	assert.Equal(t, "REBOOT=ReallySuppress /QN", c.MSI.InstallParams)
	assert.Equal(t, "/L*V", c.MSI.LoggingOptions)
	assert.Equal(t, 600, c.MSI.MutexWaitTime)
	assert.Equal(t, "PSAppDeployToolkit", c.Toolkit.CompanyName)
	assert.Equal(t, "CMTrace", c.Toolkit.LogStyle)
	assert.Equal(t, 10, c.Toolkit.LogMaxSize)
	assert.Equal(t, `HKLM:\SOFTWARE`, c.Toolkit.RegPath)
	assert.Equal(t, `HKCU:\SOFTWARE`, c.Toolkit.RegPathNoAdminRights)
	assert.Equal(t, "Fluent", c.UI.DialogStyle)
	assert.Equal(t, 1618, c.UI.DefaultExitCode)
	assert.Equal(t, 1602, c.UI.DeferExitCode)
	assert.Equal(t, 3300, c.UI.DefaultTimeout)
	assert.True(t, c.Toolkit.LogAppend)
	assert.True(t, c.UI.BalloonNotifications)
}

func TestLoadOverlayMergesSparseKeys(t *testing.T) {
	dir := t.TempDir()
	overlay := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(overlay, []byte("Toolkit:\n  CompanyName: Contoso\nUI:\n  DeferExitCode: 60012\n"), 0o644))

	c, err := Load(overlay)
	require.NoError(t, err)

	// Overridden keys.
	assert.Equal(t, "Contoso", c.Toolkit.CompanyName)
	assert.Equal(t, 60012, c.UI.DeferExitCode)
	// Untouched defaults survive.
	assert.Equal(t, "CMTrace", c.Toolkit.LogStyle)
	assert.Equal(t, 1618, c.UI.DefaultExitCode)
	assert.Equal(t, "REBOOT=ReallySuppress /QN", c.MSI.InstallParams)
}

func TestLoadMissingOverlayFails(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "absent.yaml"))
	assert.Error(t, err)
}

func TestExpandEnv(t *testing.T) {
	t.Setenv("PSADT_TEST_DIR", `C:\ProgramData`)
	assert.Equal(t, `C:\ProgramData\Logs\Software`, ExpandEnv(`$env:PSADT_TEST_DIR\Logs\Software`))
	assert.Equal(t, "no tokens", ExpandEnv("no tokens"))
}

func TestLookup(t *testing.T) {
	c, err := Default()
	require.NoError(t, err)
	v, ok := c.Lookup(`Toolkit\CompanyName`)
	require.True(t, ok)
	assert.Equal(t, "PSAppDeployToolkit", v)
	_, ok = c.Lookup(`Nope\Missing`)
	assert.False(t, ok)
}
