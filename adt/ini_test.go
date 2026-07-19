package adt

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

func TestIniValueRoundTrip(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "settings.ini")

	require.NoError(t, SetADTIniValue(ctx, SetADTIniValueOptions{
		FilePath: path,
		Section:  "General",
		Key:      "Server",
		Value:    "srv01.contoso.com",
	}))
	require.NoError(t, SetADTIniValue(ctx, SetADTIniValueOptions{
		FilePath: path,
		Section:  "General",
		Key:      "Port",
		Value:    "8443",
	}))

	v, err := GetADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "General", Key: "Server"})
	require.NoError(t, err)
	assert.Equal(t, "srv01.contoso.com", v)

	// Update in place.
	require.NoError(t, SetADTIniValue(ctx, SetADTIniValueOptions{
		FilePath: path,
		Section:  "General",
		Key:      "Server",
		Value:    "srv02.contoso.com",
	}))
	v, err = GetADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "General", Key: "Server"})
	require.NoError(t, err)
	assert.Equal(t, "srv02.contoso.com", v)

	// Remove the value; reading it afterwards reports not-found.
	require.NoError(t, RemoveADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "General", Key: "Server"}))
	_, err = GetADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "General", Key: "Server"})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	// The other key is untouched.
	v, err = GetADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "General", Key: "Port"})
	require.NoError(t, err)
	assert.Equal(t, "8443", v)
}

func TestIniValueMissingFile(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "absent.ini")

	_, err := GetADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "S", Key: "K"})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	// Removing from a missing file is not an error (PSADT parity).
	require.NoError(t, RemoveADTIniValue(ctx, IniValueOptions{FilePath: path, Section: "S", Key: "K"}))
}

func TestIniValueValidation(t *testing.T) {
	ctx := t.Context()

	_, err := GetADTIniValue(ctx, IniValueOptions{FilePath: "", Section: "S", Key: "K"})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	err = SetADTIniValue(ctx, SetADTIniValueOptions{FilePath: "x.ini", Section: "  ", Key: "K"})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	err = RemoveADTIniValue(ctx, IniValueOptions{FilePath: "x.ini", Section: "S", Key: ""})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestIniSectionMergeAndOverwrite(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "sections.ini")

	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{
		FilePath: path,
		Section:  "App",
		Content:  map[string]string{"Name": "Demo", "Version": "1.0"},
	}))

	// Merge keeps existing keys and adds/updates the supplied ones.
	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{
		FilePath: path,
		Section:  "App",
		Content:  map[string]string{"Version": "2.0", "Publisher": "ACME"},
	}))
	m, err := GetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "App"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Name": "Demo", "Version": "2.0", "Publisher": "ACME"}, m)

	// Merging empty content is a no-op (PSADT parity).
	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "App"}))
	m, err = GetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "App"})
	require.NoError(t, err)
	assert.Len(t, m, 3)

	// Overwrite replaces the section wholesale.
	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{
		FilePath:  path,
		Section:   "App",
		Content:   map[string]string{"Only": "this"},
		Overwrite: true,
	}))
	m, err = GetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "App"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"Only": "this"}, m)
}

func TestIniSectionRemove(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "remove.ini")

	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{
		FilePath: path,
		Section:  "Keep",
		Content:  map[string]string{"a": "1"},
	}))
	require.NoError(t, SetADTIniSection(ctx, IniSectionOptions{
		FilePath: path,
		Section:  "Drop",
		Content:  map[string]string{"b": "2"},
	}))

	require.NoError(t, RemoveADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "Drop"}))
	_, err := GetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "Drop"})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	m, err := GetADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "Keep"})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"a": "1"}, m)

	// Removing a missing section is not an error.
	require.NoError(t, RemoveADTIniSection(ctx, IniSectionOptions{FilePath: path, Section: "Drop"}))
}

func TestIniFileOnDiskFormat(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "format.ini")

	require.NoError(t, SetADTIniValue(ctx, SetADTIniValueOptions{FilePath: path, Section: "S", Key: "k", Value: "v"}))

	raw, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "[S]\r\nk=v\r\n", string(raw))
}
