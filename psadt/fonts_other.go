//go:build !windows

package psadt

import "fmt"

// addFontResource is Windows-only (GDI font registration); off-Windows it
// surfaces the not-Windows sentinel.
func addFontResource(path string) error {
	return fmt.Errorf("psadt: AddFontResource [%s]: %w", path, errNotWindows)
}

// removeFontResource is Windows-only; off-Windows it surfaces the not-Windows
// sentinel.
func removeFontResource(path string) error {
	return fmt.Errorf("psadt: RemoveFontResource [%s]: %w", path, errNotWindows)
}
