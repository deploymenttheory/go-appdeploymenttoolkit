package session

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/deferral"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/logging"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/wts"
)

func testDeps(t *testing.T, active bool) Deps {
	t.Helper()
	sessions := []wts.SessionInfo{}
	if active {
		sessions = append(sessions, wts.SessionInfo{SessionID: 1, UserName: "deploy", IsActive: true})
	}
	return Deps{
		Registry: regkey.NewFake(),
		WTS:      &wts.Static{Sessions: sessions, SessionID: 1},
		IsAdmin:  func() bool { return true },
		Culture:  func() string { return "en-US" },
		Now:      func() time.Time { return time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC) },
		// Deterministic deploy-mode probes: OOBE done, nothing running,
		// non-interactive station, no resolvable active-user SID.
		OobeCompleted:      func() (bool, error) { return true, nil },
		ProcessRunning:     func([]string) (bool, error) { return false, nil },
		ProcessInteractive: func() bool { return false },
		ActiveUserSID:      func() string { return "" },
	}
}

func testOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	return Options{
		AppVendor:  "Contoso",
		AppName:    "Test App",
		AppVersion: "1.0",
		AppArch:    "x64",
		ConfigOverlayPath: writeOverlay(t, dir),
	}
}

// writeOverlay redirects log paths into the test's temp dir.
func writeOverlay(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	logDir := strings.ReplaceAll(filepath.Join(dir, "logs"), `\`, `\\`)
	content := "Toolkit:\n  LogPath: " + logDir + "\n  LogPathNoAdminRights: " + logDir + "\n"
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestOpenComposesNames(t *testing.T) {
	// PSADT 4.2 Auto semantics resolve Silent when no processes-to-close are
	// running; give the session a running process so Auto lands Interactive.
	opts := testOptions(t)
	opts.AppProcessesToClose = []ProcessObject{{Name: "notepad"}}
	deps := testDeps(t, true)
	deps.ProcessRunning = func([]string) (bool, error) { return true, nil }
	s, err := Open(context.Background(), opts, deps)
	require.NoError(t, err)
	defer s.Close(context.Background())

	assert.Equal(t, "Contoso_TestApp_1.0_x64_EN_01", s.InstallName())
	assert.Equal(t, "Contoso Test App 1.0", s.InstallTitle())
	assert.Equal(t, DeployModeInteractive, s.DeployMode())
	assert.False(t, s.IsSilent())
	assert.Contains(t, s.NewLogFileName("PSAppDeployToolkit"), "Contoso_TestApp_1.0_x64_EN_01_PSAppDeployToolkit_Install.log")
}

func TestAutoModeResolvesSilentWithoutUsers(t *testing.T) {
	s, err := Open(context.Background(), testOptions(t), testDeps(t, false))
	require.NoError(t, err)
	defer s.Close(context.Background())
	assert.Equal(t, DeployModeSilent, s.DeployMode())
	assert.True(t, s.IsSilent())
}

func TestExplicitModeWins(t *testing.T) {
	opts := testOptions(t)
	opts.DeployMode = DeployModeNonInteractive
	s, err := Open(context.Background(), opts, testDeps(t, true))
	require.NoError(t, err)
	defer s.Close(context.Background())
	assert.Equal(t, DeployModeNonInteractive, s.DeployMode())
	assert.True(t, s.IsNonInteractive())
}

func TestRequireAdminRejected(t *testing.T) {
	opts := testOptions(t)
	opts.RequireAdmin = true
	deps := testDeps(t, true)
	deps.IsAdmin = func() bool { return false }
	_, err := Open(context.Background(), opts, deps)
	assert.Error(t, err)
}

func TestCloseClassifiesExitCodes(t *testing.T) {
	cases := []struct {
		name     string
		exitCode int
		want     int
		status   Status
	}{
		{"success stays zero", 0, 0, StatusComplete},
		{"reboot passes through", 3010, 3010, StatusRestartRequired},
		{"failure passes through", 1603, 1603, StatusError},
		{"defer code passes through", 1602, 1602, StatusFastRetry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Open(context.Background(), testOptions(t), testDeps(t, true))
			require.NoError(t, err)
			s.SetExitCode(tc.exitCode)
			assert.Equal(t, tc.status, s.GetDeploymentStatus())
			assert.Equal(t, tc.want, s.Close(context.Background()))
			assert.True(t, s.Closed())
		})
	}
}

func TestCloseSuppressesRebootPassThru(t *testing.T) {
	opts := testOptions(t)
	opts.SuppressRebootPassThru = true
	s, err := Open(context.Background(), opts, testDeps(t, true))
	require.NoError(t, err)
	s.SetExitCode(3010)
	assert.Equal(t, 0, s.Close(context.Background()))
}

func TestCloseWithMsiCodesNormalizes(t *testing.T) {
	s, err := Open(context.Background(), testOptions(t), testDeps(t, true))
	require.NoError(t, err)
	s.ExitWithMsiCodes = true
	s.SetExitCode(1641) // reboot code from msiexec
	got := s.Close(context.Background())
	assert.Equal(t, 3010, got)
}

func TestLogFileWritten(t *testing.T) {
	deps := testDeps(t, true)
	s, err := Open(context.Background(), testOptions(t), deps)
	require.NoError(t, err)
	s.SetInstallPhase("Install")
	s.WriteLog("hello from the test", logging.SeverityInfo, "", "")
	logPath := s.LogPath()
	s.Close(context.Background())

	data, err := os.ReadFile(logPath)
	require.NoError(t, err)
	content := string(data)
	assert.Contains(t, content, "hello from the test")
	assert.Contains(t, content, "[Install] ::")
	assert.Contains(t, content, "<![LOG[") // CMTrace by default
}

// writeCompressOverlay configures CompressLogs with temp/log paths inside the
// test's temp dir.
func writeCompressOverlay(t *testing.T, dir string, maxHistory int) string {
	t.Helper()
	p := filepath.Join(dir, "config.yaml")
	esc := func(s string) string { return strings.ReplaceAll(s, `\`, `\\`) }
	content := "Toolkit:\n" +
		"  LogPath: " + esc(filepath.Join(dir, "logs")) + "\n" +
		"  LogPathNoAdminRights: " + esc(filepath.Join(dir, "logs")) + "\n" +
		"  TempPath: " + esc(filepath.Join(dir, "temp")) + "\n" +
		"  TempPathNoAdminRights: " + esc(filepath.Join(dir, "temp")) + "\n" +
		"  CompressLogs: true\n" +
		fmt.Sprintf("  LogMaxHistory: %d\n", maxHistory)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o644))
	return p
}

func TestCompressLogsZipsAndPrunes(t *testing.T) {
	dir := t.TempDir()
	overlay := writeCompressOverlay(t, dir, 1)

	openClose := func(tick *int) {
		opts := testOptions(t)
		opts.ConfigOverlayPath = overlay
		deps := testDeps(t, true)
		base := time.Date(2026, 7, 18, 10, 0, 0, 0, time.UTC)
		deps.Now = func() time.Time { *tick++; return base.Add(time.Duration(*tick) * time.Second) }
		s, err := Open(context.Background(), opts, deps)
		require.NoError(t, err)

		// The live log is redirected into the temp capture folder.
		assert.Contains(t, s.LogPath(), filepath.Join(dir, "temp"))
		assert.NotEmpty(t, s.CompressLogDir())
		s.WriteLog("compressed entry", logging.SeverityInfo, "", "")
		capture := s.CompressLogDir()
		s.Close(context.Background())

		// Capture folder removed after zipping.
		_, err = os.Stat(capture)
		assert.True(t, os.IsNotExist(err), "capture folder should be removed after close")
	}

	tick := 0
	openClose(&tick)
	archives, err := filepath.Glob(filepath.Join(dir, "logs", "*.zip"))
	require.NoError(t, err)
	require.Len(t, archives, 1)

	// A second session with LogMaxHistory=1 prunes down to a single archive.
	openClose(&tick)
	archives, err = filepath.Glob(filepath.Join(dir, "logs", "*.zip"))
	require.NoError(t, err)
	assert.Len(t, archives, 1, "pruning should keep only LogMaxHistory archives")
}

func TestDeferHistoryLifecycle(t *testing.T) {
	s, err := Open(context.Background(), testOptions(t), testDeps(t, true))
	require.NoError(t, err)
	defer s.Close(context.Background())

	times := uint32(2)
	require.NoError(t, s.SetDeferHistory(deferral.History{TimesRemaining: &times}))
	h, err := s.DeferHistory()
	require.NoError(t, err)
	require.NotNil(t, h.TimesRemaining)
	assert.Equal(t, uint32(2), *h.TimesRemaining)
	require.NoError(t, s.ResetDeferHistory())
	h, err = s.DeferHistory()
	require.NoError(t, err)
	assert.Nil(t, h.TimesRemaining)
}

func TestCloseResetsDeferHistoryOnSuccess(t *testing.T) {
	deps := testDeps(t, true) // shared fake registry across both sessions
	s, err := Open(context.Background(), testOptions(t), deps)
	require.NoError(t, err)
	times := uint32(1)
	require.NoError(t, s.SetDeferHistory(deferral.History{TimesRemaining: &times}))
	s.Close(context.Background())

	s2, err := Open(context.Background(), testOptions(t), deps)
	require.NoError(t, err)
	defer s2.Close(context.Background())
	h, err := s2.DeferHistory()
	require.NoError(t, err)
	assert.Nil(t, h.TimesRemaining, "success close should reset deferral history")
}
