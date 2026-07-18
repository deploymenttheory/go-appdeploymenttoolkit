package psadt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalEdgeExtensions(t *testing.T) {
	t.Run("empty map", func(t *testing.T) {
		out, err := marshalEdgeExtensions(map[string]edgeExtension{})
		require.NoError(t, err)
		assert.Equal(t, "{}", out)
	})

	t.Run("single extension without minimum version", func(t *testing.T) {
		out, err := marshalEdgeExtensions(map[string]edgeExtension{
			"abc": {InstallationMode: "force_installed", UpdateURL: "https://edge.microsoft.com/crx"},
		})
		require.NoError(t, err)
		assert.Equal(t,
			`{"abc":{"installation_mode":"force_installed","update_url":"https://edge.microsoft.com/crx"}}`,
			out)
	})

	t.Run("minimum version included and keys sorted", func(t *testing.T) {
		out, err := marshalEdgeExtensions(map[string]edgeExtension{
			"zzz": {InstallationMode: "allowed", UpdateURL: "https://u/2"},
			"aaa": {InstallationMode: "force_installed", UpdateURL: "https://u/1", MinimumVersionRequired: "1.2.3"},
		})
		require.NoError(t, err)
		assert.Equal(t,
			`{"aaa":{"installation_mode":"force_installed","update_url":"https://u/1","minimum_version_required":"1.2.3"},`+
				`"zzz":{"installation_mode":"allowed","update_url":"https://u/2"}}`,
			out)
	})
}

func TestParseEdgeExtensions(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		m, err := parseEdgeExtensions("")
		require.NoError(t, err)
		assert.Empty(t, m)
	})
	t.Run("empty object", func(t *testing.T) {
		m, err := parseEdgeExtensions("{}")
		require.NoError(t, err)
		assert.Empty(t, m)
	})
	t.Run("round trip", func(t *testing.T) {
		in := `{"abc":{"installation_mode":"force_installed","update_url":"https://u","minimum_version_required":"2.0"}}`
		m, err := parseEdgeExtensions(in)
		require.NoError(t, err)
		require.Contains(t, m, "abc")
		assert.Equal(t, "force_installed", m["abc"].InstallationMode)
		assert.Equal(t, "https://u", m["abc"].UpdateURL)
		assert.Equal(t, "2.0", m["abc"].MinimumVersionRequired)
	})
	t.Run("invalid json", func(t *testing.T) {
		_, err := parseEdgeExtensions("not json")
		assert.Error(t, err)
	})
}

func TestIsWellFormedAbsoluteURL(t *testing.T) {
	assert.True(t, isWellFormedAbsoluteURL("https://edge.microsoft.com/extensionwebstorebase/v1/crx"))
	assert.False(t, isWellFormedAbsoluteURL(""))
	assert.False(t, isWellFormedAbsoluteURL("   "))
	assert.False(t, isWellFormedAbsoluteURL("/relative/path"))
	assert.False(t, isWellFormedAbsoluteURL("not a url"))
}

func TestAddADTEdgeExtensionValidation(t *testing.T) {
	installFake(t)
	ctx := context.Background()

	err := AddADTEdgeExtension(ctx, AddADTEdgeExtensionOptions{})
	assert.ErrorIs(t, err, ErrInvalidOption)

	err = AddADTEdgeExtension(ctx, AddADTEdgeExtensionOptions{
		ExtensionID: "abc", InstallationMode: "bogus", UpdateURL: "https://u",
	})
	assert.ErrorIs(t, err, ErrInvalidOption)

	err = AddADTEdgeExtension(ctx, AddADTEdgeExtensionOptions{
		ExtensionID: "abc", InstallationMode: "force_installed", UpdateURL: "not-a-url",
	})
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestAddRemoveADTEdgeExtensionRoundTrip(t *testing.T) {
	installFake(t)
	ctx := context.Background()

	require.NoError(t, AddADTEdgeExtension(ctx, AddADTEdgeExtensionOptions{
		ExtensionID:      "abcdefghijklmnop",
		InstallationMode: "force_installed",
		UpdateURL:        "https://edge.microsoft.com/crx",
	}))

	got, err := getEdgeExtensions(ctx)
	require.NoError(t, err)
	require.Contains(t, got, "abcdefghijklmnop")
	assert.Equal(t, "force_installed", got["abcdefghijklmnop"].InstallationMode)

	// Removing an extension that is not configured is a no-op.
	require.NoError(t, RemoveADTEdgeExtension(ctx, RemoveADTEdgeExtensionOptions{ExtensionID: "missing"}))

	require.NoError(t, RemoveADTEdgeExtension(ctx, RemoveADTEdgeExtensionOptions{ExtensionID: "abcdefghijklmnop"}))
	got, err = getEdgeExtensions(ctx)
	require.NoError(t, err)
	assert.NotContains(t, got, "abcdefghijklmnop")
}
