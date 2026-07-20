package logging

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func fixedEntry() Entry {
	return Entry{
		Time:          time.Date(2026, 7, 18, 14, 30, 5, 123_000_000, time.FixedZone("X", 3600)),
		Message:       "Installation started.",
		Severity:      SeverityInfo,
		Source:        "OpenADTSession",
		ScriptSection: "Pre-Install",
		Username:      `CONTOSO\deploy`,
		ProcessID:     4242,
		FileName:      "OpenADTSession",
	}
}

func TestCMTraceLine(t *testing.T) {
	got := fixedEntry().CMTraceLine()
	assert.Equal(t, `<![LOG[[Pre-Install] :: Installation started.]LOG]!>`+
		`<time="14:30:05.123+60" date="7-18-2026" component="OpenADTSession" `+
		`context="CONTOSO\deploy" type="1" thread="4242" file="OpenADTSession">`, got)
}

func TestCMTraceLineNegativeOffset(t *testing.T) {
	e := fixedEntry()
	e.Time = e.Time.In(time.FixedZone("Y", -5*3600))
	assert.Contains(t, e.CMTraceLine(), `time="08:30:05.123-300"`)
}

func TestCMTraceLineMultiline(t *testing.T) {
	e := fixedEntry()
	e.Message = "line one\n\n  indented"
	got := e.CMTraceLine()
	assert.Contains(t, got, "line one\r\n \r\n  indented\r\n]LOG]!>")
}

func TestCMTraceDividerSkipsSectionPrefix(t *testing.T) {
	e := fixedEntry()
	e.Message = LogDivider
	assert.NotContains(t, e.CMTraceLine(), "[Pre-Install] ::")
}

func TestLegacyLine(t *testing.T) {
	got := fixedEntry().LegacyLine()
	assert.Equal(t, "[2026-07-18T14:30:05.1230000+01:00] [Pre-Install] [OpenADTSession] [Info] :: Installation started.", got)
}

func TestLegacyLineNoSection(t *testing.T) {
	e := fixedEntry()
	e.ScriptSection = ""
	got := e.LegacyLine()
	assert.True(t, strings.HasPrefix(got, "[2026-07-18T14:30:05.1230000+01:00] [OpenADTSession]"))
}

func TestSeverityNames(t *testing.T) {
	assert.Equal(t, "Success", SeveritySuccess.String())
	assert.Equal(t, "Error", SeverityError.String())
	assert.Equal(t, StyleLegacy, ParseStyle("legacy"))
	assert.Equal(t, StyleCMTrace, ParseStyle("CMTrace"))
	assert.Equal(t, StyleCMTrace, ParseStyle(""))
}
