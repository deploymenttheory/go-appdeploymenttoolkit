package psadt

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

// defaultRegistryBackend returns the live Windows registry backend used by
// the sessionless registry facade functions.
func defaultRegistryBackend() regkey.Backend { return regkey.NewNative() }
