package psadt

import (
	"archive/zip"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestRemoveADTInvalidFileNameChars(t *testing.T) {
	assert.Equal(t, "abcd", RemoveADTInvalidFileNameChars(`a<b>:"/\|?*cd`))
	assert.Equal(t, "trimmed", RemoveADTInvalidFileNameChars("  trimmed  "))
	assert.Equal(t, "control", RemoveADTInvalidFileNameChars("con\x00\x1ftrol"))
	assert.Equal(t, "untouched.txt", RemoveADTInvalidFileNameChars("untouched.txt"))
}

func TestCopyADTFileSessionless(t *testing.T) {
	src := filepath.Join(t.TempDir(), "payload.txt")
	writeTestFile(t, src, "data")
	dest := filepath.Join(t.TempDir(), "Deployed")

	require.NoError(t, CopyADTFile(context.Background(), CopyADTFileOptions{
		Path:        []string{src},
		Destination: dest,
	}))
	data, err := os.ReadFile(filepath.Join(dest, "payload.txt"))
	require.NoError(t, err)
	assert.Equal(t, "data", string(data))
}

func TestCopyADTFileValidation(t *testing.T) {
	err := CopyADTFile(context.Background(), CopyADTFileOptions{Destination: "x"})
	assert.ErrorIs(t, err, ErrInvalidOption)

	err = CopyADTFile(context.Background(), CopyADTFileOptions{
		Path:         []string{"x"},
		Destination:  "y",
		FileCopyMode: "warp",
	})
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestRemoveADTFileMissingPathIsTolerated(t *testing.T) {
	err := RemoveADTFile(context.Background(), RemoveADTFileOptions{
		Path: []string{filepath.Join(t.TempDir(), "not-there.txt")},
	})
	assert.NoError(t, err)
}

func TestRemoveADTFileGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "a.log"), "a")
	writeTestFile(t, filepath.Join(dir, "b.log"), "b")
	writeTestFile(t, filepath.Join(dir, "keep.txt"), "k")

	require.NoError(t, RemoveADTFile(context.Background(), RemoveADTFileOptions{
		Path: []string{filepath.Join(dir, "*.log")},
	}))
	assert.NoFileExists(t, filepath.Join(dir, "a.log"))
	assert.NoFileExists(t, filepath.Join(dir, "b.log"))
	assert.FileExists(t, filepath.Join(dir, "keep.txt"))
}

func TestRemoveADTFileFolderRequiresRecurse(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "folder")
	writeTestFile(t, filepath.Join(dir, "file.txt"), "x")

	// Without Recurse the folder is skipped, not deleted.
	require.NoError(t, RemoveADTFile(context.Background(), RemoveADTFileOptions{
		Path: []string{dir},
	}))
	assert.DirExists(t, dir)

	require.NoError(t, RemoveADTFile(context.Background(), RemoveADTFileOptions{
		Path:    []string{dir},
		Recurse: true,
	}))
	assert.NoDirExists(t, dir)
}

func TestNewADTFolderAndRemoveADTFolder(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nested")
	require.NoError(t, NewADTFolder(context.Background(), dir))
	assert.DirExists(t, dir)
	// Idempotent when the folder exists.
	require.NoError(t, NewADTFolder(context.Background(), dir))

	writeTestFile(t, filepath.Join(dir, "sub", "file.txt"), "x")
	require.NoError(t, RemoveADTFolder(context.Background(), RemoveADTFolderOptions{Path: dir}))
	assert.NoDirExists(t, dir)

	// Missing folders are tolerated.
	require.NoError(t, RemoveADTFolder(context.Background(), RemoveADTFolderOptions{Path: dir}))
}

func TestRemoveADTFolderDisableRecursion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "folder")
	writeTestFile(t, filepath.Join(dir, "file.txt"), "x")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "empty"), 0o755))

	// Files and empty subfolders are removed, then the folder itself.
	require.NoError(t, RemoveADTFolder(context.Background(), RemoveADTFolderOptions{
		Path:             dir,
		DisableRecursion: true,
	}))
	assert.NoDirExists(t, dir)

	// A non-empty subfolder aborts the removal.
	writeTestFile(t, filepath.Join(dir, "busy", "file.txt"), "x")
	err := RemoveADTFolder(context.Background(), RemoveADTFolderOptions{
		Path:             dir,
		DisableRecursion: true,
	})
	require.Error(t, err)
	assert.DirExists(t, filepath.Join(dir, "busy"))
}

func TestNewADTZipFile(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "Payload")
	writeTestFile(t, filepath.Join(srcDir, "root.txt"), "root")
	writeTestFile(t, filepath.Join(srcDir, "sub", "leaf.txt"), "leaf")
	loose := filepath.Join(t.TempDir(), "loose.txt")
	writeTestFile(t, loose, "loose")
	dest := filepath.Join(t.TempDir(), "out.zip")

	require.NoError(t, NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:     []string{srcDir, loose},
		DestinationPath: dest,
	}))

	zr, err := zip.OpenReader(dest)
	require.NoError(t, err)
	defer zr.Close()
	var names []string
	contents := map[string]string{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		rc, err := f.Open()
		require.NoError(t, err)
		data, err := io.ReadAll(rc)
		require.NoError(t, err)
		require.NoError(t, rc.Close())
		contents[f.Name] = string(data)
	}
	sort.Strings(names)
	assert.Equal(t, []string{"Payload/root.txt", "Payload/sub/leaf.txt", "loose.txt"}, names)
	assert.Equal(t, "leaf", contents["Payload/sub/leaf.txt"])
	assert.Equal(t, "loose", contents["loose.txt"])
}

func TestNewADTZipFileOverwrite(t *testing.T) {
	src := filepath.Join(t.TempDir(), "file.txt")
	writeTestFile(t, src, "v1")
	dest := filepath.Join(t.TempDir(), "out.zip")

	require.NoError(t, NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:     []string{src},
		DestinationPath: dest,
	}))

	// Existing archive without Overwrite errors.
	err := NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:     []string{src},
		DestinationPath: dest,
	})
	assert.ErrorIs(t, err, fs.ErrExist)

	// Overwrite replaces it.
	require.NoError(t, NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:     []string{src},
		DestinationPath: dest,
		Overwrite:       true,
	}))
}

func TestNewADTZipFileRemoveSourceAfterArchiving(t *testing.T) {
	srcDir := filepath.Join(t.TempDir(), "Payload")
	writeTestFile(t, filepath.Join(srcDir, "root.txt"), "root")
	dest := filepath.Join(t.TempDir(), "out.zip")

	require.NoError(t, NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:                []string{srcDir},
		DestinationPath:            dest,
		RemoveSourceAfterArchiving: true,
	}))
	assert.NoDirExists(t, srcDir)
	assert.FileExists(t, dest)
}

func TestNewADTZipFileMissingSource(t *testing.T) {
	err := NewADTZipFile(context.Background(), NewADTZipFileOptions{
		LiteralPath:     []string{filepath.Join(t.TempDir(), "ghost")},
		DestinationPath: filepath.Join(t.TempDir(), "out.zip"),
	})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCopyADTFileToUserProfilesUsesSeam(t *testing.T) {
	profileA := t.TempDir()
	profileB := t.TempDir()
	orig := getUserProfilePaths
	t.Cleanup(func() { getUserProfilePaths = orig })
	getUserProfilePaths = func(_ context.Context, _ UserProfileFilterOptions) ([]string, error) {
		return []string{profileA, profileB}, nil
	}

	src := filepath.Join(t.TempDir(), "settings.ini")
	writeTestFile(t, src, "cfg")
	require.NoError(t, CopyADTFileToUserProfiles(context.Background(), CopyADTFileToUserProfilesOptions{
		Path:        []string{src},
		Destination: filepath.Join("AppData", "Roaming", "Vendor"),
	}))
	for _, profile := range []string{profileA, profileB} {
		assert.FileExists(t, filepath.Join(profile, "AppData", "Roaming", "Vendor", "settings.ini"))
	}
}

func TestRemoveADTFileFromUserProfilesUsesSeam(t *testing.T) {
	profile := t.TempDir()
	target := filepath.Join(profile, "AppData", "Roaming", "Vendor", "settings.ini")
	writeTestFile(t, target, "cfg")
	orig := getUserProfilePaths
	t.Cleanup(func() { getUserProfilePaths = orig })
	getUserProfilePaths = func(_ context.Context, _ UserProfileFilterOptions) ([]string, error) {
		return []string{profile}, nil
	}

	require.NoError(t,
		RemoveADTFileFromUserProfiles(context.Background(), RemoveADTFileFromUserProfilesOptions{
			Path: []string{filepath.Join("AppData", "Roaming", "Vendor", "settings.ini")},
		}))
	assert.NoFileExists(t, target)
}

func TestCopyADTContentToCacheRequiresSession(t *testing.T) {
	_, err := CopyADTContentToCache(context.Background(), "")
	assert.ErrorIs(t, err, ErrNoActiveSession)
	assert.ErrorIs(t, RemoveADTContentFromCache(context.Background()), ErrNoActiveSession)
}

func TestGetADTFileVersionOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("stub applies off Windows only")
	}
	_, err := GetADTFileVersion(context.Background(), "whatever.exe")
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
	_, err = GetADTFreeDiskSpace(context.Background(), "C:")
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
}

func TestGetADTEnvironmentVariableProcess(t *testing.T) {
	t.Setenv("PSADT_TEST_ENV", "value1")
	v, err := GetADTEnvironmentVariable(context.Background(), GetADTEnvironmentVariableOptions{
		Variable: "PSADT_TEST_ENV",
	})
	require.NoError(t, err)
	assert.Equal(t, "value1", v)

	_, err = GetADTEnvironmentVariable(context.Background(), GetADTEnvironmentVariableOptions{
		Variable: "PSADT_TEST_ENV_ABSENT",
	})
	assert.ErrorIs(t, err, ErrNotFound)

	_, err = GetADTEnvironmentVariable(context.Background(), GetADTEnvironmentVariableOptions{
		Variable: "X",
		Target:   "Universe",
	})
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestSetAndRemoveADTEnvironmentVariableProcess(t *testing.T) {
	t.Setenv("PSADT_TEST_SET", "seed") // ensures restoration after the test
	require.NoError(t, SetADTEnvironmentVariable(context.Background(),
		SetADTEnvironmentVariableOptions{Variable: "PSADT_TEST_SET", Value: "updated"}))
	assert.Equal(t, "updated", os.Getenv("PSADT_TEST_SET"))

	require.NoError(t, RemoveADTEnvironmentVariable(context.Background(),
		RemoveADTEnvironmentVariableOptions{Variable: "PSADT_TEST_SET"}))
	_, ok := os.LookupEnv("PSADT_TEST_SET")
	assert.False(t, ok)
}

func TestEnvironmentRegistryTargetsOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("registry targets are live on Windows")
	}
	_, err := GetADTEnvironmentVariable(context.Background(), GetADTEnvironmentVariableOptions{
		Variable: "Path",
		Target:   "Machine",
	})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
	err = SetADTEnvironmentVariable(context.Background(), SetADTEnvironmentVariableOptions{
		Variable: "X",
		Value:    "y",
		Target:   "User",
	})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
	err = RemoveADTEnvironmentVariable(context.Background(), RemoveADTEnvironmentVariableOptions{
		Variable: "X",
		Target:   "User",
	})
	assert.ErrorIs(t, err, winerr.ErrNotWindows)
}
