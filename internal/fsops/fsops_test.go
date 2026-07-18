package fsops

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func TestParseMode(t *testing.T) {
	m, err := ParseMode("")
	require.NoError(t, err)
	assert.Equal(t, ModeNative, m)

	m, err = ParseMode("robocopy")
	require.NoError(t, err)
	assert.Equal(t, ModeRobocopy, m)

	_, err = ParseMode("teleport")
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestCopyFileToExistingDirectory(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, src, "hello")
	dest := t.TempDir()

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{}))
	assert.Equal(t, "hello", readFile(t, filepath.Join(dest, "file.txt")))
}

func TestCopyFileToNewFolderPathCreatesFolder(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, src, "hello")
	dest := filepath.Join(t.TempDir(), "new", "folder")

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{}))
	assert.Equal(t, "hello", readFile(t, filepath.Join(dest, "file.txt")))
}

func TestCopyFileToFileDestinationCreatesParent(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, src, "hello")
	dest := filepath.Join(t.TempDir(), "sub", "renamed.txt")

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{}))
	assert.Equal(t, "hello", readFile(t, dest))
}

func TestCopyDirectoryRecurse(t *testing.T) {
	srcRoot := t.TempDir()
	src := filepath.Join(srcRoot, "Folder")
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b")
	dest := t.TempDir()

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{Recurse: true}))
	// Folder sources nest under the destination like Copy-Item.
	assert.Equal(t, "a", readFile(t, filepath.Join(dest, "Folder", "a.txt")))
	assert.Equal(t, "b", readFile(t, filepath.Join(dest, "Folder", "sub", "b.txt")))
}

func TestCopyDirectoryWithoutRecurseCreatesEmptyFolder(t *testing.T) {
	srcRoot := t.TempDir()
	src := filepath.Join(srcRoot, "Folder")
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	dest := t.TempDir()

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{}))
	fi, err := os.Stat(filepath.Join(dest, "Folder"))
	require.NoError(t, err)
	assert.True(t, fi.IsDir())
	assert.NoFileExists(t, filepath.Join(dest, "Folder", "a.txt"))
}

func TestCopyFlatten(t *testing.T) {
	srcRoot := t.TempDir()
	src := filepath.Join(srcRoot, "Folder")
	writeFile(t, filepath.Join(src, "a.txt"), "a")
	writeFile(t, filepath.Join(src, "sub", "b.txt"), "b")
	writeFile(t, filepath.Join(src, "sub", "deeper", "c.txt"), "c")
	dest := t.TempDir()

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{Flatten: true}))
	assert.Equal(t, "a", readFile(t, filepath.Join(dest, "a.txt")))
	assert.Equal(t, "b", readFile(t, filepath.Join(dest, "b.txt")))
	assert.Equal(t, "c", readFile(t, filepath.Join(dest, "c.txt")))
	assert.NoDirExists(t, filepath.Join(dest, "sub"))
}

func TestCopyGlobPattern(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "one.txt"), "1")
	writeFile(t, filepath.Join(src, "two.txt"), "2")
	writeFile(t, filepath.Join(src, "skip.dat"), "x")
	dest := t.TempDir()

	require.NoError(t,
		Copy(context.Background(), []string{filepath.Join(src, "*.txt")}, dest, CopyOptions{}))
	assert.FileExists(t, filepath.Join(dest, "one.txt"))
	assert.FileExists(t, filepath.Join(dest, "two.txt"))
	assert.NoFileExists(t, filepath.Join(dest, "skip.dat"))
}

func TestCopyMissingSourceFails(t *testing.T) {
	dest := t.TempDir()
	err := Copy(
		context.Background(),
		[]string{filepath.Join(t.TempDir(), "nope.txt")},
		dest,
		CopyOptions{},
	)
	assert.ErrorIs(t, err, winerr.ErrNotFound)
}

func TestCopyContinueOnErrorCollectsAndContinues(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "good.txt"), "ok")
	missing := filepath.Join(t.TempDir(), "missing.txt")
	dest := t.TempDir()

	err := Copy(
		context.Background(),
		[]string{missing, filepath.Join(src, "good.txt")},
		dest,
		CopyOptions{ContinueOnError: true},
	)
	assert.ErrorIs(t, err, winerr.ErrNotFound)
	// The good file must still have been copied.
	assert.Equal(t, "ok", readFile(t, filepath.Join(dest, "good.txt")))
}

func TestCopyPreservesModTime(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, src, "hello")
	stamp := time.Date(2020, 6, 15, 12, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(src, stamp, stamp))
	dest := t.TempDir()

	require.NoError(t, Copy(context.Background(), []string{src}, dest, CopyOptions{}))
	fi, err := os.Stat(filepath.Join(dest, "file.txt"))
	require.NoError(t, err)
	assert.True(t, fi.ModTime().Equal(stamp), "mod time should be preserved")
}

func TestCopyRequiresPathAndDestination(t *testing.T) {
	err := Copy(context.Background(), nil, t.TempDir(), CopyOptions{})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	err = Copy(context.Background(), []string{"x"}, " ", CopyOptions{})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestCopyRobocopyModeOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("robocopy mode is exercised on Windows")
	}
	src := filepath.Join(t.TempDir(), "file.txt")
	writeFile(t, src, "hello")
	err := Copy(context.Background(), []string{src}, t.TempDir(), CopyOptions{Mode: ModeRobocopy})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
}

func TestItemPermissionOptionsValidate(t *testing.T) {
	base := ItemPermissionOptions{
		Path:       "/tmp/thing",
		User:       `BUILTIN\Users`,
		Action:     ActionGrant,
		Permission: PermissionRead,
	}
	assert.NoError(t, base.Validate())

	missingPath := base
	missingPath.Path = " "
	assert.ErrorIs(t, missingPath.Validate(), winerr.ErrInvalidOption)

	both := base
	both.EnableInheritance = true
	both.DisableInheritance = true
	assert.ErrorIs(t, both.Validate(), winerr.ErrInvalidOption)

	enableOnly := ItemPermissionOptions{Path: "/tmp/thing", EnableInheritance: true}
	assert.NoError(t, enableOnly.Validate())

	disableOnly := ItemPermissionOptions{
		Path:               "/tmp/thing",
		DisableInheritance: true,
		PreserveAccessRules: true,
	}
	assert.NoError(t, disableOnly.Validate())

	noUser := ItemPermissionOptions{Path: "/tmp/thing"}
	assert.ErrorIs(t, noUser.Validate(), winerr.ErrInvalidOption)

	badPermission := base
	badPermission.Permission = "Fly"
	assert.ErrorIs(t, badPermission.Validate(), winerr.ErrInvalidOption)

	removeNeedsNoPermission := base
	removeNeedsNoPermission.Action = ActionRemove
	removeNeedsNoPermission.Permission = ""
	assert.NoError(t, removeNeedsNoPermission.Validate())

	badInheritance := base
	badInheritance.Inheritance = 8
	assert.ErrorIs(t, badInheritance.Validate(), winerr.ErrInvalidOption)
}

func TestPermissionAccessMasks(t *testing.T) {
	for perm, want := range map[Permission]uint32{
		PermissionFullControl:    genericAll,
		PermissionModify:         genericRead | genericWrite | genericExecute | deleteRight,
		PermissionReadAndExecute: genericRead | genericExecute,
		PermissionRead:           genericRead,
		PermissionWrite:          genericWrite,
	} {
		mask, ok := perm.AccessMask()
		assert.True(t, ok, string(perm))
		assert.Equal(t, want, mask, string(perm))
	}
	_, ok := Permission("Nope").AccessMask()
	assert.False(t, ok)
}

func TestSetItemPermissionStubOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub applies off Windows only")
	}
	err := SetItemPermission(context.Background(), ItemPermissionOptions{
		Path:       t.TempDir(),
		User:       "S-1-5-32-545",
		Action:     ActionGrant,
		Permission: PermissionRead,
	})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)

	// Validation errors surface before the platform gate.
	err = SetItemPermission(context.Background(), ItemPermissionOptions{})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestVersionAndDiskSpaceStubsOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stubs apply off Windows only")
	}
	_, err := GetFileVersion("whatever.exe")
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
	_, err = FreeDiskSpaceMB("/")
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
}

func TestCopyCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := Copy(ctx, []string{"x"}, t.TempDir(), CopyOptions{})
	assert.ErrorIs(t, err, context.Canceled)
}

func TestCopyErrorsJoinAllFailures(t *testing.T) {
	missing1 := filepath.Join(t.TempDir(), "a.txt")
	missing2 := filepath.Join(t.TempDir(), "b.txt")
	err := Copy(
		context.Background(),
		[]string{missing1, missing2},
		t.TempDir(),
		CopyOptions{ContinueOnError: true},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "a.txt")
	assert.Contains(t, err.Error(), "b.txt")
	var joined interface{ Unwrap() []error }
	require.ErrorAs(t, err, &joined)
	assert.Len(t, joined.Unwrap(), 2)
}
