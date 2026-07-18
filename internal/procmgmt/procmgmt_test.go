package procmgmt

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

func TestParseWindowStyle(t *testing.T) {
	cases := []struct {
		in   string
		want WindowStyle
		ok   bool
	}{
		{"", WindowNormal, true},
		{"Normal", WindowNormal, true},
		{"hidden", WindowHidden, true},
		{"MINIMIZED", WindowMinimized, true},
		{"Maximized", WindowMaximized, true},
		{"fullscreen", WindowNormal, false},
	}
	for _, tc := range cases {
		got, ok := ParseWindowStyle(tc.in)
		assert.Equal(t, tc.want, got, "input %q", tc.in)
		assert.Equal(t, tc.ok, ok, "input %q", tc.in)
	}
}

func TestLaunchOptionsValidate(t *testing.T) {
	assert.ErrorIs(t, LaunchOptions{}.Validate(), winerr.ErrInvalidOption, "missing FilePath")
	assert.ErrorIs(t,
		LaunchOptions{FilePath: "x", WindowStyle: WindowStyle(99)}.Validate(),
		winerr.ErrInvalidOption, "bad WindowStyle")
	assert.ErrorIs(t,
		LaunchOptions{FilePath: "x", Timeout: -time.Second}.Validate(),
		winerr.ErrInvalidOption, "negative Timeout")
	assert.NoError(t, LaunchOptions{FilePath: "x"}.Validate())
}

func TestSplitArguments(t *testing.T) {
	assert.Nil(t, SplitArguments(""))
	assert.Equal(t, []string{"/S", "/v"}, SplitArguments("/S /v"))
	assert.Equal(t,
		[]string{"/i", `C:\My Files\app.msi`, "/qn"},
		SplitArguments(`/i "C:\My Files\app.msi" /qn`))
	assert.Equal(t, []string{""}, SplitArguments(`""`), "explicit empty argument survives")
	assert.Equal(t, []string{"a", "b c", "d"}, SplitArguments(`a "b c"  d`))
}

func TestExecLauncherExitCodeAndStreams(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable launcher test uses /bin/sh")
	}
	res, err := ExecLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:     "/bin/sh",
		ArgumentList: `-c "echo out; echo err 1>&2; exit 3"`,
	})
	require.NoError(t, err)
	assert.Equal(t, 3, res.ExitCode)
	assert.Equal(t, "out\n", res.StdOut)
	assert.Equal(t, "err\n", res.StdErr)
}

func TestExecLauncherTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable launcher test uses /bin/sh")
	}
	_, err := ExecLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:     "/bin/sh",
		ArgumentList: `-c "sleep 10"`,
		Timeout:      50 * time.Millisecond,
	})
	assert.ErrorIs(t, err, winerr.ErrTimeout)
}

func TestExecLauncherNoWait(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("portable launcher test uses /bin/sh")
	}
	res, err := ExecLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath:     "/bin/sh",
		ArgumentList: `-c "exit 7"`,
		NoWait:       true,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode, "NoWait returns before the exit code exists")
}

func TestExecLauncherLaunchFailure(t *testing.T) {
	_, err := ExecLauncher{}.Launch(context.Background(), LaunchOptions{
		FilePath: filepath.Join(t.TempDir(), "does-not-exist"),
	})
	assert.Error(t, err)
}

func TestRetryExhaustsAttempts(t *testing.T) {
	calls := 0
	wantErr := errors.New("transient")
	err := Retry(context.Background(), 3, time.Millisecond, func(context.Context) error {
		calls++
		return wantErr
	})
	assert.ErrorIs(t, err, wantErr)
	assert.Equal(t, 4, calls, "PSADT semantics: 1 initial attempt + 3 retries")
}

func TestRetrySucceedsMidway(t *testing.T) {
	calls := 0
	err := Retry(context.Background(), 5, time.Millisecond, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.New("transient")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestRetryContextCancelDuringDelay(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	err := Retry(ctx, 100, time.Hour, func(context.Context) error {
		calls++
		return errors.New("transient")
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, calls, "cancellation interrupts the inter-attempt sleep")
}

func TestRetryNilFunction(t *testing.T) {
	assert.ErrorIs(t, Retry(context.Background(), 1, time.Millisecond, nil), winerr.ErrInvalidOption)
}

// writeMinimalPE writes the smallest parseable PE image: an MS-DOS header
// whose e_lfanew points at a PE signature plus COFF file header with the
// given machine type, no optional header and no sections.
func writeMinimalPE(t *testing.T, machine uint16) string {
	t.Helper()
	// 64-byte DOS header + PE signature + 20-byte COFF header, padded to
	// 128 bytes because debug/pe reads a 96-byte DOS header region.
	buf := make([]byte, 128)
	copy(buf, "MZ")
	binary.LittleEndian.PutUint32(buf[0x3C:], 64) // e_lfanew
	copy(buf[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(buf[68:], machine)
	// NumberOfSections, SizeOfOptionalHeader etc. stay zero.
	path := filepath.Join(t.TempDir(), "test.exe")
	require.NoError(t, os.WriteFile(path, buf, 0o644))
	return path
}

func TestPEFileArchitecture(t *testing.T) {
	cases := []struct {
		machine uint16
		want    string
	}{
		{0x014C, "x86"},
		{0x8664, "x64"},
		{0xAA64, "ARM64"},
		{0x01C4, "ARM"},
		{0x0200, "IA64"},
	}
	for _, tc := range cases {
		got, err := PEFileArchitecture(writeMinimalPE(t, tc.machine))
		require.NoError(t, err, "machine 0x%04X", tc.machine)
		assert.Equal(t, tc.want, got)
	}
}

func TestPEFileArchitectureUnknownMachine(t *testing.T) {
	_, err := PEFileArchitecture(writeMinimalPE(t, 0xBEEF))
	assert.ErrorIs(t, err, winerr.ErrNotFound)
}

func TestPEFileArchitectureNotPE(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-a-pe.txt")
	require.NoError(t, os.WriteFile(path, []byte("plain text"), 0o644))
	_, err := PEFileArchitecture(path)
	assert.Error(t, err)

	_, err = PEFileArchitecture(filepath.Join(t.TempDir(), "missing.exe"))
	assert.Error(t, err)
}

func TestProcessesBySessionID(t *testing.T) {
	procs := []RunningProcess{
		{Name: "excel", PID: 100, SessionID: 1},
		{Name: "winword", PID: 200, SessionID: 2},
		{Name: "outlook", PID: 300, SessionID: 1},
	}
	got := ProcessesBySessionID(procs, 1)
	require.Len(t, got, 2)
	assert.Equal(t, uint32(100), got[0].PID)
	assert.Equal(t, uint32(300), got[1].PID)
	assert.Empty(t, ProcessesBySessionID(procs, 9))
}

func TestNormalizeProcessName(t *testing.T) {
	assert.Equal(t, "excel", normalizeProcessName("EXCEL.EXE"))
	assert.Equal(t, "excel", normalizeProcessName("excel"))
	assert.Equal(t, "Excel", trimExeSuffix("Excel.EXE"))
	assert.Equal(t, "Excel", trimExeSuffix("Excel"))
}
