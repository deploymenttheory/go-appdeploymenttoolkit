package manifest

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadValidMinimal(t *testing.T) {
	m, issues, err := Load(filepath.Join("testdata", "valid", "minimal.yaml"))
	require.NoError(t, err)
	assert.Empty(t, issues)
	assert.Equal(t, APIVersion, m.APIVersion)
	assert.Equal(t, KindDeployment, m.Kind)
	name, _ := m.Session.String("appName")
	assert.Equal(t, "Minimal", name)
	steps := m.PhaseSteps("install")
	require.Len(t, steps, 1)
	assert.Equal(t, "msi.install", steps[0].Uses)
	path, _ := steps[0].With.String("path")
	assert.Equal(t, "Minimal.msi", path)
}

func TestLoadValidFull(t *testing.T) {
	m, issues, err := Load(filepath.Join("testdata", "valid", "full.yaml"))
	require.NoError(t, err)
	assert.Empty(t, issues, "the full fixture must be schema-clean: %+v", issues)

	// Every phase slot exists; the populated ones carry their steps.
	assert.Len(t, m.Phases, 9)
	assert.Len(t, m.PhaseSteps("install"), 3)
	welcome := m.PhaseSteps("preInstall")[0]
	assert.Equal(t, "dialog.welcome", welcome.Uses)
	assert.Equal(t, "Close apps and offer deferral", welcome.DisplayName)
	patch := m.PhaseSteps("install")[1]
	assert.True(t, patch.ContinueOnError)

	// Semantic layer: full fixture stays warning-free too.
	issues = Validate(m, ValidateOptions{})
	for _, i := range issues {
		assert.NotEqual(t, SeverityError, i.Severity, "unexpected error: %+v", i)
	}
}

func TestLoadSyntaxErrorIsTerminal(t *testing.T) {
	_, _, err := Parse("inline.yaml", []byte("a: [unclosed\n"))
	assert.Error(t, err)
}

func TestLoadBrokenFixture(t *testing.T) {
	m, issues, err := Load(filepath.Join("testdata", "invalid", "broken.yaml"))
	require.NoError(t, err)
	issues = append(issues, Validate(m, ValidateOptions{})...)
	SortIssues(issues)

	wantCodes := map[string]bool{
		CodeBadEnum:         false, // apiVersion adt/v9, windowStyle hiden, priorityClass turbo
		CodeUnknownField:    false, // extras, appVersio, phase "instal"
		CodeBadType:         false, // disableLogging: maybe, arguments: [a, b]
		CodeUnknownStep:     false, // msi.instal
		CodeMissingRequired: false, // msi.install without path
		CodeBadDuration:     false, // timeout: 90
		CodeSemantic:        false, // deferTimes without allowDefer (warning)
	}
	for _, i := range issues {
		if _, tracked := wantCodes[i.Code]; tracked {
			wantCodes[i.Code] = true
		}
	}
	for code, seen := range wantCodes {
		assert.True(t, seen, "expected an issue with code %s; got %+v", code, issues)
	}

	// Spot-check positions: apiVersion on line 1, unknown step on line 13.
	for _, i := range issues {
		switch {
		case i.Path == "apiVersion":
			assert.Equal(t, 1, i.Pos.Line)
		case i.Code == CodeUnknownStep:
			assert.Equal(t, 13, i.Pos.Line)
			assert.Contains(t, i.Message, `did you mean "msi.install"`)
		}
	}

	// The report JSON round-trips and is invalid.
	report := NewReport(m.Path, issues)
	assert.False(t, report.Valid)
	blob, err := json.Marshal(report)
	require.NoError(t, err)
	var back Report
	require.NoError(t, json.Unmarshal(blob, &back))
	assert.Equal(t, len(report.Issues), len(back.Issues))
}

func TestLoadUnknownPhaseSuggestion(t *testing.T) {
	src := `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  instal:
    - uses: gpo.update
`
	_, issues, err := Parse("inline.yaml", []byte(src))
	require.NoError(t, err)
	found := false
	for _, i := range issues {
		if i.Code == CodeUnknownField && i.Path == "phases.instal" {
			found = true
			assert.Contains(t, i.Message, `did you mean "install"`)
		}
	}
	assert.True(t, found, "issues: %+v", issues)
}

func TestLoadStepEnvelopeStrictness(t *testing.T) {
	src := `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  install:
    - uses: gpo.update
      continueOnErr: true
`
	_, issues, err := Parse("inline.yaml", []byte(src))
	require.NoError(t, err)
	require.NotEmpty(t, issues)
	assert.Equal(t, CodeUnknownField, issues[0].Code)
	assert.Contains(t, issues[0].Message, `did you mean "continueOnError"`)
}
