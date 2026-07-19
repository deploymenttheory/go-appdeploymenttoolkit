package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestExampleConfigLoads guards the worked config overlay shipped under
// examples/ so a broken sample can't reach the repo: it must parse through the
// loader, apply its overrides, and leave untouched keys at their defaults.
func TestExampleConfigLoads(t *testing.T) {
	c, err := Load("../../examples/interactive-install/Config/config.yaml")
	require.NoError(t, err)

	// Overridden keys.
	assert.Equal(t, "VideoLAN", c.Toolkit.CompanyName)
	assert.True(t, c.Toolkit.LogToSubfolder)
	assert.Equal(t, "Fluent", c.UI.DialogStyle)
	assert.Equal(t, uint32(0xFFFF8800), c.UI.FluentAccentColor)
	assert.Equal(t, 1200, c.UI.DefaultTimeout)
	assert.Equal(t, 300, c.MSI.MutexWaitTime)

	// An untouched key keeps its default.
	assert.Equal(t, 1602, c.UI.DeferExitCode)
}
