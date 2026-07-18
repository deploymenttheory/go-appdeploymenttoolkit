//go:build !windows

package msipkg

// enumerateWowNode reports true on non-Windows builds so the portable tests
// exercise the WOW6432Node enumeration path against regkey.Fake.
func enumerateWowNode() bool { return true }
