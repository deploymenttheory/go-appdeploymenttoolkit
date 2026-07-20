//go:build !windows

package session

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/wts"
)

// Non-Windows builds (tests and tooling) default to inert fakes.

func defaultRegistry() regkey.Backend { return regkey.NewFake() }

func defaultWTS() wts.Query { return &wts.Static{} }

func defaultIsAdmin() bool { return false }

func defaultCulture() string { return "" }

func defaultOobeCompleted() (bool, error) { return true, nil }

func defaultProcessRunning([]string) (bool, error) { return false, nil }

func defaultProcessInteractive() bool { return false }

func defaultActiveUserSID() string { return "" }
