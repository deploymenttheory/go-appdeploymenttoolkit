package shortcut

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestURLShortcutRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.url")
	in := &Shortcut{
		Path:         path,
		TargetPath:   "https://example.com/app",
		IconLocation: `C:\Windows\System32\shell32.dll`,
		IconIndex:    5,
	}
	require.NoError(t, Create(in))

	out, err := Read(path)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/app", out.TargetPath)
	assert.Equal(t, `C:\Windows\System32\shell32.dll`, out.IconLocation)
	assert.Equal(t, 5, out.IconIndex)
	assert.True(t, out.IsURL())
}

func TestUpdateURLShortcut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "site.url")
	require.NoError(t, Create(&Shortcut{Path: path, TargetPath: "https://old.example"}))
	require.NoError(t, Update(path, func(s *Shortcut) { s.TargetPath = "https://new.example" }))

	out, err := Read(path)
	require.NoError(t, err)
	assert.Equal(t, "https://new.example", out.TargetPath)
}

func TestCreateRequiresPathAndTarget(t *testing.T) {
	assert.Error(t, Create(&Shortcut{Path: "x.url"}))
	assert.Error(t, Create(&Shortcut{TargetPath: "y"}))
}

func TestReadMissingReturnsNotFound(t *testing.T) {
	_, err := Read(filepath.Join(t.TempDir(), "absent.url"))
	assert.Error(t, err)
}

func TestParseWindowStyle(t *testing.T) {
	for in, want := range map[string]WindowStyle{
		"":          WindowStyleNormal,
		"Normal":    WindowStyleNormal,
		"maximized": WindowStyleMaximized,
		"MINIMIZED": WindowStyleMinimized,
	} {
		got, err := ParseWindowStyle(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseWindowStyle("sideways")
	assert.Error(t, err)
}

func TestIsURL(t *testing.T) {
	assert.True(t, (&Shortcut{Path: `C:\a\b.url`}).IsURL())
	assert.False(t, (&Shortcut{Path: `C:\a\b.lnk`}).IsURL())
}
