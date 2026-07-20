package winadt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsWindowsSubsystem(t *testing.T) {
	assert.True(t, isWindowsSubsystem(imageSubsystemWindowsGUI))
	assert.False(t, isWindowsSubsystem(3)) // IMAGE_SUBSYSTEM_WINDOWS_CUI (console)
	assert.False(t, isWindowsSubsystem(0)) // IMAGE_SUBSYSTEM_UNKNOWN
}
