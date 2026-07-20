package adt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/procmgmt"
)

func TestResolveTokenSelection(t *testing.T) {
	sel, err := resolveTokenSelection(StartADTProcessAsUserOptions{})
	require.NoError(t, err)
	assert.Equal(t, procmgmt.TokenDefault, sel)

	sel, err = resolveTokenSelection(StartADTProcessAsUserOptions{UseLinkedAdminToken: true})
	require.NoError(t, err)
	assert.Equal(t, procmgmt.TokenLinkedAdmin, sel)

	sel, err = resolveTokenSelection(StartADTProcessAsUserOptions{UseHighestAvailableToken: true})
	require.NoError(t, err)
	assert.Equal(t, procmgmt.TokenHighestAvailable, sel)

	sel, err = resolveTokenSelection(StartADTProcessAsUserOptions{UseUnelevatedToken: true})
	require.NoError(t, err)
	assert.Equal(t, procmgmt.TokenUnelevated, sel)

	_, err = resolveTokenSelection(StartADTProcessAsUserOptions{
		UseLinkedAdminToken: true,
		UseUnelevatedToken:  true,
	})
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestDecodeStreamOEM(t *testing.T) {
	// Plain ASCII is identical in every OEM code page.
	assert.Equal(t, "hello", decodeStream("hello", "oem"))
}
