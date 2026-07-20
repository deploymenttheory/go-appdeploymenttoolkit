package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func touch(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o644))
}

func TestDiscoverZeroConfigMSIPrefersUnsuffixed(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "app.x64.msi"))
	touch(t, filepath.Join(dir, "app.msi"))
	got, err := discoverZeroConfigMSI(dir)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "app.msi"), got)
}

func TestDiscoverTransforms(t *testing.T) {
	dir := t.TempDir()
	msi := filepath.Join(dir, "app.msi")
	touch(t, msi)
	assert.Nil(t, discoverTransforms(msi), "no MST present")

	touch(t, filepath.Join(dir, "app.mst"))
	assert.Equal(t, []string{filepath.Join(dir, "app.mst")}, discoverTransforms(msi))
}

func TestDiscoverPatchesSortedAlphabetically(t *testing.T) {
	dir := t.TempDir()
	touch(t, filepath.Join(dir, "zz-later.msp"))
	touch(t, filepath.Join(dir, "aa-first.MSP"))
	touch(t, filepath.Join(dir, "not-a-patch.txt"))
	got := discoverPatches(dir)
	require.Len(t, got, 2)
	assert.Equal(t, filepath.Join(dir, "aa-first.MSP"), got[0])
	assert.Equal(t, filepath.Join(dir, "zz-later.msp"), got[1])
}
