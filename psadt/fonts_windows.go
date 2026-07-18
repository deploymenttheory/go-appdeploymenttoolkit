//go:build windows

package psadt

import (
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/gdi"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/ui/windowsandmessaging"
)

// addFontResource registers a font file with GDI (AddFontResourceW) and
// broadcasts WM_FONTCHANGE so running applications pick up the new font,
// mirroring PSADT's FontUtilities.AddFont.
func addFontResource(path string) error {
	if gdi.AddFontResource(path) == 0 {
		return fmt.Errorf("psadt: AddFontResource [%s]: %w", path, lastFontError())
	}
	broadcastFontChange()
	return nil
}

// removeFontResource unregisters a font file with GDI (RemoveFontResourceW)
// and broadcasts WM_FONTCHANGE, mirroring PSADT's FontUtilities.RemoveFont.
func removeFontResource(path string) error {
	if !gdi.RemoveFontResource(path) {
		return fmt.Errorf("psadt: RemoveFontResource [%s]: %w", path, lastFontError())
	}
	broadcastFontChange()
	return nil
}

// lastFontError returns the last Windows error, or a not-found sentinel when
// the GDI call cleared the error state.
func lastFontError() error {
	if err := windows.GetLastError(); err != nil {
		return err
	}
	return winerr.ErrNotFound
}

// broadcastFontChange notifies all top-level windows that the pool of
// installed fonts changed (best-effort).
func broadcastFontChange() {
	//nolint:errcheck // best-effort notification; failures are inconsequential
	_, _ = windowsandmessaging.SendMessage(
		windowsandmessaging.HWND_BROADCAST,
		windowsandmessaging.WM_FONTCHANGE,
		0,
		0,
	)
}
