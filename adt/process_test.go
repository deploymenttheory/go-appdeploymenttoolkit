package adt

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

// shellExit builds portable StartADTProcessOptions that exit with the given
// code via /bin/sh. Tests using it must skip on Windows.
func shellExit(code string) StartADTProcessOptions {
	return StartADTProcessOptions{
		FilePath:     "/bin/sh",
		ArgumentList: `-c "exit ` + code + `"`,
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("portable process tests use /bin/sh")
	}
}

func TestStartADTProcessSuccess(t *testing.T) {
	skipOnWindows(t)
	res, err := StartADTProcess(context.Background(), shellExit("0"))
	require.NoError(t, err)
	assert.Equal(t, 0, res.ExitCode)
}

func TestStartADTProcessFailureReturnsExitError(t *testing.T) {
	skipOnWindows(t)
	res, err := StartADTProcess(context.Background(), shellExit("7"))
	require.Error(t, err)
	ee, ok := AsExitError(err)
	require.True(t, ok, "failure exit codes surface as *ExitError")
	assert.Equal(t, 7, ee.Code)
	require.NotNil(t, res, "result accompanies the classification error")
	assert.Equal(t, 7, res.ExitCode)
}

func TestStartADTProcessRebootCodeIsSuccess(t *testing.T) {
	skipOnWindows(t)
	_, err := StartADTProcess(context.Background(), shellExit("30"))
	require.Error(t, err, "30 is a failure by default")

	opts := shellExit("30")
	opts.RebootExitCodes = []int{30} // ...but a reboot code when listed
	res, err := StartADTProcess(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 30, res.ExitCode)
}

func TestStartADTProcessIgnoreExitCodes(t *testing.T) {
	skipOnWindows(t)

	opts := shellExit("7")
	opts.IgnoreExitCodes = []string{"*"}
	res, err := StartADTProcess(context.Background(), opts)
	require.NoError(t, err, "wildcard ignores every exit code")
	assert.Equal(t, 7, res.ExitCode)

	opts.IgnoreExitCodes = []string{"5", "7"}
	_, err = StartADTProcess(context.Background(), opts)
	require.NoError(t, err, "listed code is ignored")

	opts.IgnoreExitCodes = []string{"5"}
	_, err = StartADTProcess(context.Background(), opts)
	require.Error(t, err, "unlisted code still fails")
}

func TestStartADTProcessCustomSuccessCodes(t *testing.T) {
	skipOnWindows(t)
	opts := shellExit("7")
	opts.SuccessExitCodes = []int{0, 7}
	res, err := StartADTProcess(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 7, res.ExitCode)
}

func TestStartADTProcessInvalidWindowStyle(t *testing.T) {
	opts := StartADTProcessOptions{FilePath: "x", WindowStyle: "fullscreen"}
	_, err := StartADTProcess(context.Background(), opts)
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestStartADTProcessTimeout(t *testing.T) {
	skipOnWindows(t)
	opts := StartADTProcessOptions{
		FilePath:     "/bin/sh",
		ArgumentList: `-c "sleep 10"`,
		Timeout:      50 * time.Millisecond,
	}
	_, err := StartADTProcess(context.Background(), opts)
	assert.ErrorIs(t, err, ErrTimeout)
}

func TestStartADTProcessNoWait(t *testing.T) {
	skipOnWindows(t)
	opts := shellExit("9")
	opts.NoWait = true
	res, err := StartADTProcess(context.Background(), opts)
	require.NoError(t, err, "NoWait never classifies an exit code")
	assert.Equal(t, 0, res.ExitCode)
}

func TestStartADTProcessSessionExitCodePassback(t *testing.T) {
	skipOnWindows(t)
	sessOpts := testSessionOptions(t)
	// Unix exit codes are mod-256, so model a reboot code below 256.
	sessOpts.AppRebootExitCodes = []int{100, 1641, 3010}
	s, err := OpenADTSession(context.Background(), sessOpts)
	require.NoError(t, err)
	defer CloseADTSession(context.Background(), s)

	// Reboot code with default lists flags the session exit code.
	_, err = StartADTProcess(context.Background(), shellExit("100"))
	require.NoError(t, err)
	assert.Equal(t, 100, s.ExitCode())

	// Failure codes always stick.
	_, err = StartADTProcess(context.Background(), shellExit("7"))
	require.Error(t, err)
	assert.Equal(t, 7, s.ExitCode())
}

func TestStartADTProcessExplicitCodesSkipSessionPassback(t *testing.T) {
	skipOnWindows(t)
	s, err := OpenADTSession(context.Background(), testSessionOptions(t))
	require.NoError(t, err)
	defer CloseADTSession(context.Background(), s)

	opts := shellExit("42")
	opts.SuccessExitCodes = []int{42}
	_, err = StartADTProcess(context.Background(), opts)
	require.NoError(t, err)
	assert.Equal(t, 0, s.ExitCode(),
		"explicit code lists disable session exit-code passback (PSADT canSetExitCode)")
}

func TestStartADTProcessResolvesDirFiles(t *testing.T) {
	skipOnWindows(t)
	opts := testSessionOptions(t)
	dirFiles := t.TempDir()
	script := filepath.Join(dirFiles, "tool.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755))
	opts.ScriptDirectory = filepath.Dir(dirFiles)
	opts.DirFiles = dirFiles

	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	defer CloseADTSession(context.Background(), s)

	res, err := StartADTProcess(context.Background(), StartADTProcessOptions{FilePath: "tool.sh"})
	require.NoError(t, err, "relative FilePath resolves against the session DirFiles")
	assert.Equal(t, 0, res.ExitCode)
}

func TestStartADTProcessAsUserNotWindows(t *testing.T) {
	skipOnWindows(t)
	_, err := StartADTProcessAsUser(context.Background(), StartADTProcessAsUserOptions{
		StartADTProcessOptions: shellExit("0"),
	})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
}

func TestGetADTRunningProcessesValidationAndPlatform(t *testing.T) {
	_, err := GetADTRunningProcesses(context.Background(), nil)
	assert.ErrorIs(t, err, ErrInvalidOption)

	if runtime.GOOS != "windows" {
		_, err = GetADTRunningProcesses(context.Background(), []ProcessObject{{Name: "excel"}})
		assert.ErrorIs(t, err, winerr.ErrNotWindows)
	}
}

func TestGetADTWindowTitleValidationAndPlatform(t *testing.T) {
	_, err := GetADTWindowTitle(context.Background(), GetADTWindowTitleOptions{})
	assert.ErrorIs(t, err, ErrInvalidOption)

	if runtime.GOOS != "windows" {
		_, err = GetADTWindowTitle(context.Background(), GetADTWindowTitleOptions{GetAllWindowTitles: true})
		assert.ErrorIs(t, err, winerr.ErrNotWindows)
	}
}

func TestTestADTMutexAvailabilityValidationAndPlatform(t *testing.T) {
	_, err := TestADTMutexAvailability(context.Background(), TestADTMutexAvailabilityOptions{})
	assert.ErrorIs(t, err, ErrInvalidOption)

	if runtime.GOOS != "windows" {
		_, err = TestADTMutexAvailability(context.Background(),
			TestADTMutexAvailabilityOptions{MutexName: `Global\_MSIExecute`})
		assert.ErrorIs(t, err, winerr.ErrNotWindows)
	}
}

// buildTestPE writes a minimal PE image with the given COFF machine type.
func buildTestPE(t *testing.T, machine uint16) string {
	t.Helper()
	// Padded to 128 bytes: debug/pe reads a 96-byte DOS header region.
	buf := make([]byte, 128)
	copy(buf, "MZ")
	binary.LittleEndian.PutUint32(buf[0x3C:], 64)
	copy(buf[64:], "PE\x00\x00")
	binary.LittleEndian.PutUint16(buf[68:], machine)
	path := filepath.Join(t.TempDir(), "facade.exe")
	require.NoError(t, os.WriteFile(path, buf, 0o644))
	return path
}

func TestGetADTPEFileArchitecture(t *testing.T) {
	arch, err := GetADTPEFileArchitecture(context.Background(), buildTestPE(t, 0x8664))
	require.NoError(t, err)
	assert.Equal(t, "x64", arch)

	arch, err = GetADTPEFileArchitecture(context.Background(), buildTestPE(t, 0x014C))
	require.NoError(t, err)
	assert.Equal(t, "x86", arch)

	_, err = GetADTPEFileArchitecture(context.Background(), filepath.Join(t.TempDir(), "nope.exe"))
	assert.Error(t, err)
}

func TestInvokeADTCommandWithRetries(t *testing.T) {
	// Immediate success never sleeps.
	calls := 0
	err := InvokeADTCommandWithRetries(context.Background(),
		InvokeADTCommandWithRetriesOptions{Attempts: 3, DelaySeconds: 1},
		func(context.Context) error {
			calls++
			return nil
		})
	require.NoError(t, err)
	assert.Equal(t, 1, calls)

	// Cancellation interrupts the inter-attempt sleep after the first
	// failure (retry-count exhaustion itself is covered in procmgmt).
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	calls = 0
	transient := errors.New("transient")
	err = InvokeADTCommandWithRetries(ctx,
		InvokeADTCommandWithRetriesOptions{Attempts: 5, DelaySeconds: 30},
		func(context.Context) error {
			calls++
			return transient
		})
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Equal(t, 1, calls)

	assert.ErrorIs(t,
		InvokeADTCommandWithRetries(context.Background(), InvokeADTCommandWithRetriesOptions{}, nil),
		ErrInvalidOption)
}
