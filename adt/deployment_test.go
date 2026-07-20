package adt

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

)

func testSessionOptions(t *testing.T) SessionOptions {
	t.Helper()
	dir := t.TempDir()
	logDir := strings.ReplaceAll(filepath.Join(dir, "logs"), `\`, `\\`)
	overlay := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(overlay,
		[]byte("Toolkit:\n  LogPath: "+logDir+"\n  LogPathNoAdminRights: "+logDir+"\n"), 0o644))
	return SessionOptions{
		AppVendor:         "Contoso",
		AppName:           "Runner Test",
		AppVersion:        "1.0",
		ConfigOverlayPath: overlay,
	}
}

func runDeployment(t *testing.T, d *Deployment, args ...string) int {
	t.Helper()
	code := -1
	d.Args = append([]string{}, args...) // non-nil so os.Args (test flags) is never parsed
	d.Exit = func(c int) { code = c }
	d.Run(context.Background())
	return code
}

func TestRunHappyPathPhases(t *testing.T) {
	var order []string
	d := &Deployment{
		Session: testSessionOptions(t),
		PreInstall: func(ctx context.Context, s *DeploymentSession) error {
			order = append(order, "pre:"+s.InstallPhase())
			return nil
		},
		Install: func(ctx context.Context, s *DeploymentSession) error {
			order = append(order, "main:"+s.InstallPhase())
			return nil
		},
		PostInstall: func(ctx context.Context, s *DeploymentSession) error {
			order = append(order, "post:"+s.InstallPhase())
			return nil
		},
	}
	code := runDeployment(t, d)
	assert.Equal(t, 0, code)
	assert.Equal(t, []string{"pre:Pre-Install", "main:Install", "post:Post-Install"}, order)
	assert.False(t, TestADTSessionActive(), "session stack should be empty after Run")
}

func TestRunUninstallFlagDispatch(t *testing.T) {
	ran := ""
	d := &Deployment{
		Session:   testSessionOptions(t),
		Install:   func(ctx context.Context, s *DeploymentSession) error { ran = "install"; return nil },
		Uninstall: func(ctx context.Context, s *DeploymentSession) error { ran = "uninstall"; return nil },
	}
	code := runDeployment(t, d, "-DeploymentType", "Uninstall", "-DeployMode", "Silent")
	assert.Equal(t, 0, code)
	assert.Equal(t, "uninstall", ran)
}

func TestRunPhaseErrorMapsToGenericFailure(t *testing.T) {
	d := &Deployment{
		Session: testSessionOptions(t),
		Install: func(ctx context.Context, s *DeploymentSession) error {
			return errors.New("installer exploded")
		},
	}
	assert.Equal(t, ExitCodeGenericFailure, runDeployment(t, d))
}

func TestRunExitErrorCarriesCode(t *testing.T) {
	d := &Deployment{
		Session: testSessionOptions(t),
		Install: func(ctx context.Context, s *DeploymentSession) error {
			return NewExitError(1603, errors.New("msi failure"))
		},
	}
	assert.Equal(t, 1603, runDeployment(t, d))
}

func TestRunDeferredMapsToDeferExitCode(t *testing.T) {
	d := &Deployment{
		Session: testSessionOptions(t),
		Install: func(ctx context.Context, s *DeploymentSession) error {
			return ErrDeferred
		},
	}
	// Default UI.DeferExitCode is 1602; FastRetry classification passes it through.
	assert.Equal(t, 1602, runDeployment(t, d))
}

func TestRunPanicRecovery(t *testing.T) {
	d := &Deployment{
		Session: testSessionOptions(t),
		Install: func(ctx context.Context, s *DeploymentSession) error {
			panic("boom")
		},
	}
	assert.Equal(t, ExitCodeRunnerFailure, runDeployment(t, d))
}

func TestRunSkipsNilPhases(t *testing.T) {
	d := &Deployment{Session: testSessionOptions(t)}
	assert.Equal(t, 0, runDeployment(t, d))
}

func TestRunDefaultsScriptDirectoryToExecutableDir(t *testing.T) {
	opts := testSessionOptions(t) // leaves ScriptDirectory empty
	var gotDirFiles string
	opts.Hooks.Opening = append(opts.Hooks.Opening, func(ctx context.Context, s *DeploymentSession) error {
		gotDirFiles = s.DirFiles()
		return nil
	})
	d := &Deployment{Session: opts}
	require.Equal(t, 0, runDeployment(t, d))

	exe, err := os.Executable()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(filepath.Dir(exe), "Files"), gotDirFiles,
		"an unset ScriptDirectory should default DirFiles beside the executable")
}

func TestRunKeepsExplicitScriptDirectory(t *testing.T) {
	opts := testSessionOptions(t)
	opts.ScriptDirectory = t.TempDir()
	var gotDirFiles string
	opts.Hooks.Opening = append(opts.Hooks.Opening, func(ctx context.Context, s *DeploymentSession) error {
		gotDirFiles = s.DirFiles()
		return nil
	})
	d := &Deployment{Session: opts}
	require.Equal(t, 0, runDeployment(t, d))
	assert.Equal(t, filepath.Join(opts.ScriptDirectory, "Files"), gotDirFiles)
}

func TestOnLogEntryHookReceivesEntries(t *testing.T) {
	opts := testSessionOptions(t)
	var messages []string
	opts.Hooks.OnLogEntry = append(opts.Hooks.OnLogEntry, func(e LogEntry) {
		messages = append(messages, e.Message)
	})
	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	s.WriteLog("hook probe", LogSeverityInfo, "Test", "")
	CloseADTSession(context.Background(), s)

	assert.Contains(t, messages, "hook probe")
	assert.NotEmpty(t, messages, "opening entries should also reach the hook")
}

func TestSessionFacadeFunctions(t *testing.T) {
	opts := testSessionOptions(t)
	var hookOrder []string
	opts.Hooks.Starting = append(opts.Hooks.Starting, func(ctx context.Context) error {
		hookOrder = append(hookOrder, "starting")
		return nil
	})
	opts.Hooks.Opening = append(opts.Hooks.Opening, func(ctx context.Context, s *DeploymentSession) error {
		hookOrder = append(hookOrder, "opening")
		return nil
	})
	opts.Hooks.Finishing = append(opts.Hooks.Finishing, func(ctx context.Context) error {
		hookOrder = append(hookOrder, "finishing")
		return nil
	})

	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, TestADTSessionActive())

	got, err := GetADTSession()
	require.NoError(t, err)
	assert.Same(t, s, got)

	cfg, err := GetADTConfig()
	require.NoError(t, err)
	assert.Equal(t, "PSAppDeployToolkit", cfg.Toolkit.CompanyName)

	tbl, err := GetADTStringTable()
	require.NoError(t, err)
	assert.Equal(t, "Restart Required", tbl.MustGet("RestartPrompt.Title", ""))

	env, err := GetADTEnvironmentTable()
	require.NoError(t, err)
	assert.Equal(t, "PSAppDeployToolkit", env.AppDeployToolkitName)

	require.NoError(t, WriteADTLogEntry(context.Background(), LogEntryOptions{Message: []string{"facade log"}}))
	name, err := NewADTLogFileName("Custom")
	require.NoError(t, err)
	assert.Contains(t, name, "_Custom_")

	CloseADTSession(context.Background(), s)
	assert.False(t, TestADTSessionActive())
	assert.Equal(t, []string{"starting", "opening", "finishing"}, hookOrder)

	_, err = GetADTSession()
	assert.ErrorIs(t, err, ErrNoActiveSession)
}
