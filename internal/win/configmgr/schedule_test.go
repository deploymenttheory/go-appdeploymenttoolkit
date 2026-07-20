package configmgr

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleGUID(t *testing.T) {
	// The .NET Guid(byte[]{0×14, high, low}).ToString("b") layout.
	assert.Equal(t, "{00000000-0000-0000-0000-000000000113}", SoftwareUpdatesScan.ScheduleGUID())
	assert.Equal(t, "{00000000-0000-0000-0000-000000000001}", HardwareInventory.ScheduleGUID())
	assert.Equal(t, "{00000000-0000-0000-0000-000000000121}", ApplicationManagerPolicyAction.ScheduleGUID())
	assert.Equal(t, "{00000000-0000-0000-0000-000000000021}", RequestMachinePolicy.ScheduleGUID())
}

func TestParseScheduleID(t *testing.T) {
	cases := map[string]ScheduleID{
		"SoftwareUpdatesScan":  SoftwareUpdatesScan,
		"hardwareinventory":    HardwareInventory,
		"RequestMachinePolicy": RequestMachinePolicy,
	}
	for in, want := range cases {
		got, err := ParseScheduleID(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	_, err := ParseScheduleID("NotARealTask")
	assert.Error(t, err)
}
