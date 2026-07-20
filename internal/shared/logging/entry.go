// Package logging implements PSAppDeployToolkit's log formats: CMTrace
// (OneTrace-compatible) and Legacy text, with size-based rotation. Formatting
// is a byte-level port of PSADT's LogEntry.cs and is fully portable; only the
// console echo is platform-aware.
package logging

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Severity mirrors PSADT's LogSeverity enum.
type Severity int

// Severity values (numeric parity with PSADT's LogSeverity).
const (
	SeveritySuccess Severity = 0
	SeverityInfo    Severity = 1
	SeverityWarning Severity = 2
	SeverityError   Severity = 3
)

func (s Severity) String() string {
	switch s {
	case SeveritySuccess:
		return "Success"
	case SeverityInfo:
		return "Info"
	case SeverityWarning:
		return "Warning"
	case SeverityError:
		return "Error"
	default:
		return "Info"
	}
}

// Style selects the log file format.
type Style int

// Style values.
const (
	StyleCMTrace Style = iota
	StyleLegacy
)

// ParseStyle maps config.psd1 LogStyle values to a Style.
func ParseStyle(s string) Style {
	if strings.EqualFold(s, "Legacy") {
		return StyleLegacy
	}
	return StyleCMTrace
}

// LogDivider is the horizontal-rule message PSADT writes around sessions;
// CMTrace lines never prefix it with the script section.
const LogDivider = "-------------------------------------------------------------------------------"

// leadingSpace is the punctuation-space character PSADT substitutes for
// leading whitespace so OneTrace doesn't trim indented lines.
const leadingSpace = '\u2008'

var firstNonSpace = regexp.MustCompile(`[^\s]`)

// Entry is one log record; a direct port of PSADT's LogEntry.
type Entry struct {
	Time          time.Time
	Message       string
	Severity      Severity
	Source        string // CMTrace "component", Legacy "[source]"
	ScriptSection string // e.g. the session's current InstallPhase
	Username      string // CMTrace "context"
	ProcessID     int    // CMTrace "thread" (PSADT uses caller PID here)
	FileName      string // CMTrace "file"
}

// LegacyLine renders the PSADT Legacy format:
// [<ISO-8601 round-trip stamp>] [<section>] [<source>] [<severity>] :: <message>
func (e Entry) LegacyLine() string {
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(e.Time.Format("2006-01-02T15:04:05.0000000-07:00"))
	b.WriteByte(']')
	if e.ScriptSection != "" {
		b.WriteString(" [" + e.ScriptSection + "]")
	}
	b.WriteString(" [" + e.Source + "] [" + e.Severity.String() + "] :: " + e.Message)
	return b.String()
}

// CMTraceLine renders the CMTrace/OneTrace format, replicating PSADT's
// multiline handling: blank lines become a punctuation space and leading
// whitespace runs are replaced with punctuation spaces of equal length.
//
// Parity note: PSADT emits the timezone's base (non-DST) UTC offset; Go does
// not expose the base offset, so the entry time's actual offset is used.
func (e Entry) CMTraceLine() string {
	msg := e.Message
	if strings.Contains(msg, "\n") {
		lines := strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n")
		for i, m := range lines {
			if strings.TrimSpace(m) == "" {
				lines[i] = string(leadingSpace)
			} else if start := firstNonSpace.FindStringIndex(m)[0]; start > 0 {
				lines[i] = strings.Repeat(string(leadingSpace), start) + m[start:]
			}
		}
		msg = strings.Join(lines, "\r\n") + "\r\n"
	}
	section := ""
	if e.ScriptSection != "" && e.Message != LogDivider {
		section = "[" + e.ScriptSection + "] :: "
	}
	_, offsetSeconds := e.Time.Zone()
	offsetMinutes := offsetSeconds / 60
	sign := ""
	if offsetMinutes >= 0 {
		sign = "+"
	}
	return fmt.Sprintf(
		`<![LOG[%s%s]LOG]!><time="%s%s%s" date="%s" component="%s" context="%s" type="%d" thread="%d" file="%s">`,
		section,
		msg,
		e.Time.Format("15:04:05.000"),
		sign,
		strconv.Itoa(offsetMinutes),
		e.Time.Format("1-02-2006"),
		e.Source,
		e.Username,
		e.Severity,
		e.ProcessID,
		e.FileName,
	)
}

// Line renders the entry in the given style.
func (e Entry) Line(style Style) string {
	if style == StyleLegacy {
		return e.LegacyLine()
	}
	return e.CMTraceLine()
}
