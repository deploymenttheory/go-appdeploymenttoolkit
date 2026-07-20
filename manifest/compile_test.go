package manifest

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

// testStepCalls records invocations of the test-only steps registered below.
var testStepCalls []string

func init() {
	// Test-only steps exercising the compile chain without touching the OS.
	register(StepSpec{
		Name: "test.record", Summary: "test helper", Platforms: []Platform{PlatformWindows, PlatformDarwin},
		Params: []ParamSpec{{Name: "tag", Type: TypeString, Required: true, Description: "recorded tag"}},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			tag := p.StringOr("tag", "")
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				testStepCalls = append(testStepCalls, tag)
				return nil
			}, nil
		},
	})
	register(StepSpec{
		Name: "test.fail", Summary: "test helper", Platforms: []Platform{PlatformWindows, PlatformDarwin},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				testStepCalls = append(testStepCalls, "fail")
				return errors.New("step exploded")
			}, nil
		},
	})
}

// testPackageDir builds a scratch package with a log-redirecting config
// overlay so sessions open cleanly.
func testPackageDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Files"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "Config"), 0o755))
	logDir := strings.ReplaceAll(filepath.Join(dir, "logs"), `\`, `\\`)
	overlay := "Toolkit:\n  LogPath: " + logDir + "\n  LogPathNoAdminRights: " + logDir + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Config", "config.yaml"), []byte(overlay), 0o644))
	return dir
}

func TestCompileFullFixture(t *testing.T) {
	pkg := testPackageDir(t)
	// Provide the files the fixture references so layer (c) is clean.
	for _, f := range []string{"Widget.msi", "Widget.mst", "Widget-hotfix.msp", "setup-helper.exe"} {
		require.NoError(t, os.WriteFile(filepath.Join(pkg, "Files", f), []byte("x"), 0o644))
	}
	m, issues, err := Load(filepath.Join("testdata", "valid", "full.yaml"))
	require.NoError(t, err)
	require.Empty(t, issues)

	dep, err := Compile(m, CompileOptions{PackageDir: pkg})
	require.NoError(t, err)
	assert.NotNil(t, dep.PreInstall)
	assert.NotNil(t, dep.Install)
	assert.NotNil(t, dep.PostInstall)
	assert.NotNil(t, dep.Uninstall)
	assert.NotNil(t, dep.Repair)
	assert.Equal(t, "Widget", dep.Session.AppName)
	assert.Equal(t, pkg, dep.Session.ScriptDirectory)
	assert.Len(t, dep.Session.AppProcessesToClose, 1)
}

func TestCompileRefusesInvalid(t *testing.T) {
	m, _, err := Load(filepath.Join("testdata", "invalid", "broken.yaml"))
	require.NoError(t, err)
	_, err = Compile(m, CompileOptions{PackageDir: t.TempDir()})
	assert.Error(t, err)
}

func TestCompiledPhaseChainsAndContinueOnError(t *testing.T) {
	pkg := testPackageDir(t)
	m, issues, err := Parse("inline.yaml", []byte(`
apiVersion: v0.1.0-alpha
kind: Deployment
session: {appName: ChainTest, noProcessDetection: true}
phases:
  install:
    - uses: test.record
      with: {tag: first}
    - uses: test.fail
      name: allowed to fail
      continueOnError: true
    - uses: test.record
      with: {tag: after-continue}
    - uses: test.fail
    - uses: test.record
      with: {tag: never}
`))
	require.NoError(t, err)
	require.Empty(t, issues)

	dep, err := Compile(m, CompileOptions{PackageDir: pkg})
	require.NoError(t, err)
	require.NotNil(t, dep.Install)

	s, err := adt.OpenADTSession(context.Background(), dep.Session)
	require.NoError(t, err)
	defer adt.CloseADTSession(context.Background(), s)

	testStepCalls = nil
	err = dep.Install(context.Background(), s)
	require.Error(t, err, "the second test.fail is not continueOnError")
	assert.Contains(t, err.Error(), "test.fail")
	assert.Equal(t, []string{"first", "fail", "after-continue", "fail"}, testStepCalls,
		"continueOnError keeps the chain going; a hard failure stops it")
}

func TestNormalizePackagePaths(t *testing.T) {
	pkg := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(pkg, "Files"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(pkg, "Files", "a.msi"), []byte("x"), 0o644))

	spec, ok := Lookup("msi.install")
	require.True(t, ok)
	p := Params{}
	p.set("path", Value{V: "a.msi"})
	p = normalizePackagePaths(p, spec, pkg)
	got, _ := p.String("path")
	assert.Equal(t, filepath.Join(pkg, "Files", "a.msi"), got)

	// GUIDs and absolute paths pass through.
	p = Params{}
	p.set("path", Value{V: "{26923b43-4d38-484f-9b9e-de460746276c}"})
	p = normalizePackagePaths(p, spec, pkg)
	got, _ = p.String("path")
	assert.Equal(t, "{26923b43-4d38-484f-9b9e-de460746276c}", got)
}
