package procmgmt

import "strings"

// ParseTimeoutAction parses Start-ADTProcess's -TimeoutAction: "" or "Error"
// (terminate-and-error, the default) vs "Continue" (suppress the timeout
// error).
func ParseTimeoutAction(s string) (continueOnTimeout, ok bool) {
	switch strings.ToLower(s) {
	case "", "error":
		return false, true
	case "continue":
		return true, true
	default:
		return false, false
	}
}

// ValidStreamEncoding reports whether a StreamEncoding value is supported:
// "" (UTF-8 passthrough), "utf-16le" or "oem".
func ValidStreamEncoding(s string) bool {
	switch strings.ToLower(s) {
	case "", "utf-16le", "oem":
		return true
	default:
		return false
	}
}
