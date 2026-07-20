package winadt

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildGpUpdateArgs(t *testing.T) {
	assert.Equal(t, "/Target:Computer /Force", buildGpUpdateArgs("Computer", true))
	assert.Equal(t, "/Target:User /Force", buildGpUpdateArgs("User", true))
	assert.Equal(t, "/Target:Computer", buildGpUpdateArgs("Computer", false))
}

func TestBuildTerminalServerArgs(t *testing.T) {
	assert.Equal(t, "User /Install", buildTerminalServerArgs("Install"))
	assert.Equal(t, "User /Execute", buildTerminalServerArgs("Execute"))
}

func TestTerminalServerModeVerb(t *testing.T) {
	assert.Equal(t, "install", terminalServerModeVerb("Install"))
	assert.Equal(t, "execute", terminalServerModeVerb("Execute"))
}
