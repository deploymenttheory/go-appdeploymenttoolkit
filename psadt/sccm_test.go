package psadt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

func TestNormalizeKB(t *testing.T) {
	assert.Equal(t, "KB2549864", normalizeKB("kb2549864"))
	assert.Equal(t, "KB2549864", normalizeKB("2549864"))
	assert.Equal(t, "KB2549864", normalizeKB(" KB2549864 "))
	assert.Equal(t, "", normalizeKB(""))
}

func TestHotfixInstalled(t *testing.T) {
	fake := regkey.NewFake()
	base := cbsPackagesKey
	require.NoError(t, fake.CreateKey("HKLM", base+`\Package_for_KB2549864~31bf3856ad364e35~amd64~~10.0.1.0`))
	require.NoError(t, fake.CreateKey("HKLM", base+`\Package_for_KB5000001~31bf3856ad364e35~amd64~~10.0.1.0`))

	found, err := hotfixInstalled(fake, "KB2549864")
	require.NoError(t, err)
	assert.True(t, found)

	found, err = hotfixInstalled(fake, "2549864") // prefix added
	require.NoError(t, err)
	assert.True(t, found)

	found, err = hotfixInstalled(fake, "KB9999999")
	require.NoError(t, err)
	assert.False(t, found)
}
