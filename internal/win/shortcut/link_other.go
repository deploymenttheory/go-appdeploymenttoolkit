//go:build !windows

package shortcut

import "github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"

// .lnk shell links require the Windows shell; only .url files work portably.

func writeLinkFile(*Shortcut) error {
	return winerr.Wrap("shortcut: .lnk files", winerr.ErrNotWindows)
}

func readLinkFile(string) (*Shortcut, error) {
	return nil, winerr.Wrap("shortcut: .lnk files", winerr.ErrNotWindows)
}
