//go:build !windows

package session

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
)

// Non-Windows builds (tests and tooling) default to inert fakes.

func defaultRegistry() regkey.Backend { return regkey.NewFake() }

func defaultWTS() wts.Query { return &wts.Static{} }

func defaultIsAdmin() bool { return false }

func defaultCulture() string { return "" }
