//go:build !windows

package psadt

import (
	"sync"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

// fallbackRegistry is a process-wide in-memory registry so that non-Windows
// builds (tests and tooling) observe their own writes across facade calls.
var fallbackRegistry = sync.OnceValue(func() regkey.Backend { return regkey.NewFake() })

// defaultRegistryBackend returns an inert in-memory backend on non-Windows
// platforms, where there is no live registry.
func defaultRegistryBackend() regkey.Backend { return fallbackRegistry() }
