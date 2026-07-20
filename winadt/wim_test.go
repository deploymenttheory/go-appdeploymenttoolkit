package winadt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildDismMountArgs(t *testing.T) {
	t.Run("by index", func(t *testing.T) {
		got := buildDismMountArgs(MountADTWimFileOptions{
			ImagePath: `C:\img\install.wim`,
			Path:      `C:\mount`,
			Index:     1,
		})
		assert.Equal(t,
			`/Mount-Wim /WimFile:"C:\img\install.wim" /MountDir:"C:\mount" /Index:1 /ReadOnly /CheckIntegrity`,
			got)
	})

	t.Run("by name", func(t *testing.T) {
		got := buildDismMountArgs(MountADTWimFileOptions{
			ImagePath: `C:\img\install.wim`,
			Path:      `C:\mount`,
			Name:      "Windows 10 Pro",
		})
		assert.Equal(t,
			`/Mount-Wim /WimFile:"C:\img\install.wim" /MountDir:"C:\mount" /Name:"Windows 10 Pro" /ReadOnly /CheckIntegrity`,
			got)
	})
}

func TestBuildDismUnmountArgs(t *testing.T) {
	assert.Equal(t, `/Unmount-Wim /MountDir:"C:\mount" /Discard`, buildDismUnmountArgs(`C:\mount`, false))
	assert.Equal(t, `/Unmount-Wim /MountDir:"C:\mount" /Commit`, buildDismUnmountArgs(`C:\mount`, true))
}

func TestValidateMountOptions(t *testing.T) {
	cases := []struct {
		name string
		opts MountADTWimFileOptions
		ok   bool
	}{
		{"index only", MountADTWimFileOptions{ImagePath: "a", Path: "b", Index: 1}, true},
		{"name only", MountADTWimFileOptions{ImagePath: "a", Path: "b", Name: "n"}, true},
		{"both index and name", MountADTWimFileOptions{ImagePath: "a", Path: "b", Index: 1, Name: "n"}, false},
		{"neither", MountADTWimFileOptions{ImagePath: "a", Path: "b"}, false},
		{"missing image", MountADTWimFileOptions{Path: "b", Index: 1}, false},
		{"missing path", MountADTWimFileOptions{ImagePath: "a", Index: 1}, false},
		{"negative index", MountADTWimFileOptions{ImagePath: "a", Path: "b", Index: -2}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateMountOptions(tc.opts)
			if tc.ok {
				require.NoError(t, err)
			} else {
				assert.ErrorIs(t, err, ErrInvalidOption)
			}
		})
	}
}
