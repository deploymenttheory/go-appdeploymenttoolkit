//go:build !windows

package adt

import "fmt"

// addFontResource is Windows-only (GDI font registration); off-Windows it
// surfaces the not-Windows sentinel.
func addFontResource(path string) error {
	return fmt.Errorf("adt: AddFontResource [%s]: %w", path, errNotWindows)
}

// removeFontResource is Windows-only; off-Windows it surfaces the not-Windows
// sentinel.
func removeFontResource(path string) error {
	return fmt.Errorf("adt: RemoveFontResource [%s]: %w", path, errNotWindows)
}
