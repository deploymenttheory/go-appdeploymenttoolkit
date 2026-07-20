package svcmgmt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStartMode(t *testing.T) {
	cases := map[string]StartMode{
		"automatic":                 StartModeAutomatic,
		"Automatic (Delayed Start)": StartModeAutomaticDelayed,
		"MANUAL":                    StartModeManual,
		"Disabled":                  StartModeDisabled,
	}
	for in, want := range cases {
		got, err := ParseStartMode(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseStartMode("bogus")
	assert.Error(t, err)
}
