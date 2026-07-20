package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadInline parses inline YAML and requires it schema-clean.
func loadInline(t *testing.T, src string) *Manifest {
	t.Helper()
	m, issues, err := Parse("inline.yaml", []byte(src))
	require.NoError(t, err)
	require.Empty(t, issues, "fixture must be schema-clean: %+v", issues)
	return m
}

func TestValidateSemanticWarnings(t *testing.T) {
	m := loadInline(t, `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  preInstall:
    - uses: dialog.welcome
      with: {deferTimes: 3}
  install:
    - uses: process.start
      with: {filePath: setup.exe, timeoutAction: continue}
`)
	issues := Validate(m, ValidateOptions{})
	byPath := map[string]Issue{}
	for _, i := range issues {
		byPath[i.Path] = i
	}
	warn, ok := byPath["phases.preInstall[0].with.deferTimes"]
	require.True(t, ok, "issues: %+v", issues)
	assert.Equal(t, SeverityWarning, warn.Severity)
	assert.Equal(t, 8, warn.Pos.Line, "anchored at the parameter value")

	warn, ok = byPath["phases.install[0].with.timeout"]
	require.True(t, ok)
	assert.Equal(t, SeverityWarning, warn.Severity)
}

func TestValidateSemanticErrors(t *testing.T) {
	m := loadInline(t, `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  uninstall:
    - uses: app.uninstall
      with: {applicationType: msi}
  install:
    - uses: process.startAsUser
      with:
        filePath: setup.exe
        useLinkedAdminToken: true
        useUnelevatedToken: true
    - uses: registry.set
      with: {key: 'HKLM:\X', name: v, value: -5, type: dword}
`)
	issues := Validate(m, ValidateOptions{})
	codes := map[string]int{}
	for _, i := range issues {
		if i.Severity == SeverityError {
			codes[i.Code]++
		}
	}
	assert.GreaterOrEqual(t, codes[CodeSemantic], 3,
		"app.uninstall missing filters, exclusive tokens, dword range: %+v", issues)
}

func TestValidatePackageDir(t *testing.T) {
	pkg := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(pkg, "Files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkg, "Files", "Widget.msi"), []byte("x"), 0o644))

	m := loadInline(t, `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  install:
    - uses: msi.install
      with:
        path: Widget.msi
        transforms: [Missing.mst]
    - uses: msi.uninstall
      with: {path: "{26923b43-4d38-484f-9b9e-de460746276c}"}
    - uses: file.copy
      with:
        path: ['C:\absolute\skipped.txt', 'glob-*.txt']
        destination: 'C:\target'
`)
	issues := Validate(m, ValidateOptions{PackageDir: pkg})
	var missing []Issue
	for _, i := range issues {
		if i.Code == CodeMissingFile {
			missing = append(missing, i)
		}
	}
	require.Len(t, missing, 1, "only the missing transform should be flagged: %+v", issues)
	assert.Contains(t, missing[0].Message, "Missing.mst")
	assert.Equal(t, "phases.install[0].with.transforms[0]", missing[0].Path)
}

func TestValidatePlatformTarget(t *testing.T) {
	m := loadInline(t, `
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: X}
phases:
  install:
    - uses: gpo.update
`)
	issues := Validate(m, ValidateOptions{Target: PlatformDarwin})
	require.NotEmpty(t, issues)
	assert.Equal(t, CodePlatformUnsupported, issues[0].Code)
	assert.Contains(t, issues[0].Message, "darwin")

	assert.Empty(t, Validate(m, ValidateOptions{Target: PlatformWindows}))
}
