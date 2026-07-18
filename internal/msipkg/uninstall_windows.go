package msipkg

import "runtime"

// enumerateWowNode reports whether the 32-bit (WOW6432Node) uninstall view
// should also be enumerated, mirroring Get-ADTApplication's
// [System.Environment]::Is64BitProcess gate.
func enumerateWowNode() bool {
	switch runtime.GOARCH {
	case "amd64", "arm64":
		return true
	default:
		return false
	}
}
