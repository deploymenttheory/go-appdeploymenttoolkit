package strtab

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaultTable(t *testing.T) {
	tbl, err := Load("", "")
	require.NoError(t, err)

	v, ok := tbl.Get("CloseAppsPrompt.Fluent.ButtonRightText", "")
	require.True(t, ok)
	assert.Equal(t, "Defer", v)

	// Per-deployment-type leaf resolution.
	v, ok = tbl.Get("BalloonTip.Start", "Install")
	require.True(t, ok)
	assert.Equal(t, "Installation started.", v)
	v, ok = tbl.Get("BalloonTip.Start", "Uninstall")
	require.True(t, ok)
	assert.Equal(t, "Uninstallation started.", v)
}

func TestLoadGermanOverlay(t *testing.T) {
	tbl, err := Load("de", "")
	require.NoError(t, err)
	v, ok := tbl.Get("BalloonTip.Start", "Install")
	require.True(t, ok)
	assert.NotEqual(t, "Installation started.", v, "German table should override the default text")
}

func TestCultureChainParentFallback(t *testing.T) {
	assert.Equal(t, []string{"pt-BR", "pt"}, cultureChain("pt-BR"))
	assert.Equal(t, []string{"de"}, cultureChain("de"))
	assert.Nil(t, cultureChain(""))

	// Unknown culture falls back to defaults without error.
	tbl, err := Load("xx-XX", "")
	require.NoError(t, err)
	v, ok := tbl.Get("RestartPrompt.Title", "")
	require.True(t, ok)
	assert.Equal(t, "Restart Required", v)
}

func TestAllEmbeddedCulturesParse(t *testing.T) {
	cultures := Cultures()
	assert.Len(t, cultures, 26)
	for _, c := range cultures {
		tbl, err := Load(c, "")
		require.NoError(t, err, "culture %s", c)
		_, ok := tbl.Get("RestartPrompt.Title", "")
		assert.True(t, ok, "culture %s missing RestartPrompt.Title", c)
	}
}

func TestMustGetReturnsPathWhenMissing(t *testing.T) {
	tbl, err := Load("", "")
	require.NoError(t, err)
	assert.Equal(t, "No.Such.Path", tbl.MustGet("No.Such.Path", ""))
}

func TestInterpolate(t *testing.T) {
	got := Interpolate(`{Toolkit\CompanyName} - App Installation`, func(ref string) (string, bool) {
		if ref == `Toolkit\CompanyName` {
			return "Contoso", true
		}
		return "", false
	})
	assert.Equal(t, "Contoso - App Installation", got)

	// Unresolvable references stay literal.
	got = Interpolate("{0} left as-is", func(string) (string, bool) { return "", false })
	assert.Equal(t, "{0} left as-is", got)
}
