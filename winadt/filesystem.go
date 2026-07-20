package winadt

import (
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/archive"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/fsops"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// errFolderNotEmpty reports a non-recursive folder removal that found
// non-empty subfolders (parity with Remove-ADTFolder's IOException).
var errFolderNotEmpty = errors.New("adt: folder not empty")

// logToSession writes a log entry to the active session, silently doing
// nothing when no session is open (these functions work sessionless).
func logToSession(message string, severity LogSeverity, source string) {
	s, err := GetADTSession()
	if err != nil {
		return
	}
	s.WriteLog(message, severity, source, "")
}

// CopyADTFileOptions mirrors the parameters of Copy-ADTFile.
type CopyADTFileOptions struct {
	// Path lists the files or folders to copy; glob patterns supported.
	Path []string
	// Destination receives the copies.
	Destination string
	// Recurse copies files in subdirectories.
	Recurse bool
	// Flatten copies all files from all subtrees into the destination root.
	Flatten bool
	// ContinueFileCopyOnError collects per-file errors and continues; the
	// joined error is returned at the end.
	ContinueFileCopyOnError bool
	// FileCopyMode is "Native" or "Robocopy". Empty defaults to the active
	// session's config (Toolkit.FileCopyMode), else "Native".
	FileCopyMode string
}

// CopyADTFile is the Go port of Copy-ADTFile: it copies files and
// directories from a source to a destination, supporting recursion,
// flattening and continue-on-error semantics.
func CopyADTFile(ctx context.Context, opts CopyADTFileOptions) error {
	if len(opts.Path) == 0 || strings.TrimSpace(opts.Destination) == "" {
		return winerr.Wrap("CopyADTFile: Path and Destination are required", winerr.ErrInvalidOption)
	}
	mode := opts.FileCopyMode
	if mode == "" {
		if s, err := GetADTSession(); err == nil {
			mode = s.Config().Toolkit.FileCopyMode
		}
	}
	copyMode, err := fsops.ParseMode(mode)
	if err != nil {
		return err
	}
	logToSession(
		fmt.Sprintf("Copying file(s) in path [%s] to destination [%s].",
			strings.Join(opts.Path, ", "), opts.Destination),
		LogSeverityInfo, "Copy-ADTFile",
	)
	err = fsops.Copy(ctx, opts.Path, opts.Destination, fsops.CopyOptions{
		Recurse:         opts.Recurse,
		Flatten:         opts.Flatten,
		ContinueOnError: opts.ContinueFileCopyOnError,
		Mode:            copyMode,
	})
	if err != nil {
		logToSession(
			fmt.Sprintf("Failed to copy file(s) to destination [%s]: %v", opts.Destination, err),
			LogSeverityWarning, "Copy-ADTFile",
		)
		return err
	}
	logToSession("File copy completed successfully.", LogSeverityInfo, "Copy-ADTFile")
	return nil
}

// RemoveADTFileOptions mirrors the parameters of Remove-ADTFile.
type RemoveADTFileOptions struct {
	// Path lists files or folders to delete; glob patterns supported.
	// Paths that do not resolve are logged and skipped, not errors.
	Path []string
	// Recurse deletes folders recursively; without it folders are skipped.
	Recurse bool
}

// RemoveADTFile is the Go port of Remove-ADTFile: it removes one or more
// files or folders, tolerating unresolvable paths.
func RemoveADTFile(ctx context.Context, opts RemoveADTFileOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	var errs []error
	for _, pattern := range opts.Path {
		items, ok := resolveRemovalPaths(pattern)
		if !ok {
			continue
		}
		for _, item := range items {
			if err := removeOne(item, opts.Recurse); err != nil {
				logToSession(
					fmt.Sprintf("Failed to delete items in path [%s]: %v", item, err),
					LogSeverityError, "Remove-ADTFile",
				)
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// resolveRemovalPaths expands a pattern, logging and skipping when nothing
// resolves (Remove-ADTFile warns instead of failing).
func resolveRemovalPaths(pattern string) ([]string, bool) {
	if strings.ContainsAny(pattern, "*?[") {
		matches, err := filepath.Glob(pattern)
		if err != nil || len(matches) == 0 {
			logToSession(
				fmt.Sprintf("Unable to resolve the path [%s] because it does not exist.", pattern),
				LogSeverityWarning, "Remove-ADTFile",
			)
			return nil, false
		}
		return matches, true
	}
	if _, err := os.Lstat(pattern); err != nil {
		logToSession(
			fmt.Sprintf("Unable to resolve the path [%s] because it does not exist.", pattern),
			LogSeverityWarning, "Remove-ADTFile",
		)
		return nil, false
	}
	return []string{pattern}, true
}

// removeOne deletes a single resolved item honoring the Recurse switch.
func removeOne(item string, recurse bool) error {
	fi, err := os.Lstat(item)
	if err != nil {
		return fmt.Errorf("adt: reading %s: %w", item, err)
	}
	if fi.IsDir() {
		if !recurse {
			logToSession(
				fmt.Sprintf("Skipping folder [%s] because the Recurse switch was not specified.", item),
				LogSeverityInfo, "Remove-ADTFile",
			)
			return nil
		}
		logToSession(
			fmt.Sprintf("Deleting file(s) recursively in path [%s]...", item),
			LogSeverityInfo, "Remove-ADTFile",
		)
		if err := os.RemoveAll(item); err != nil {
			return fmt.Errorf("adt: deleting %s: %w", item, err)
		}
		return nil
	}
	logToSession(fmt.Sprintf("Deleting file in path [%s]...", item), LogSeverityInfo, "Remove-ADTFile")
	if err := os.Remove(item); err != nil {
		return fmt.Errorf("adt: deleting %s: %w", item, err)
	}
	return nil
}

// NewADTFolder is the Go port of New-ADTFolder: it creates the folder (and
// any missing parents), succeeding silently when it already exists.
func NewADTFolder(ctx context.Context, path string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if strings.TrimSpace(path) == "" {
		return winerr.Wrap("NewADTFolder: a path is required", winerr.ErrInvalidOption)
	}
	if fi, err := os.Stat(path); err == nil && fi.IsDir() {
		logToSession(fmt.Sprintf("Folder [%s] already exists.", path), LogSeverityInfo, "New-ADTFolder")
		return nil
	}
	logToSession(fmt.Sprintf("Creating folder [%s].", path), LogSeverityInfo, "New-ADTFolder")
	if err := os.MkdirAll(path, 0o755); err != nil { //#nosec G301 -- parity with New-Item defaults
		return fmt.Errorf("adt: creating folder %s: %w", path, err)
	}
	return nil
}

// RemoveADTFolderOptions mirrors the parameters of Remove-ADTFolder.
type RemoveADTFolderOptions struct {
	// Path is the folder to remove (missing folders are logged, not errors).
	Path string
	// DisableRecursion deletes only the folder's own files and empty
	// subfolders, erroring when non-empty subfolders remain.
	DisableRecursion bool
}

// RemoveADTFolder is the Go port of Remove-ADTFolder: it removes a folder
// recursively (default) or non-recursively.
func RemoveADTFolder(ctx context.Context, opts RemoveADTFolderOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	fi, err := os.Stat(opts.Path)
	if err != nil || !fi.IsDir() {
		logToSession(
			fmt.Sprintf("Folder [%s] does not exist.", opts.Path),
			LogSeverityInfo, "Remove-ADTFolder",
		)
		return nil
	}
	if !opts.DisableRecursion {
		logToSession(
			fmt.Sprintf("Deleting folder [%s] recursively...", opts.Path),
			LogSeverityInfo, "Remove-ADTFolder",
		)
		if err := os.RemoveAll(opts.Path); err != nil {
			return fmt.Errorf("adt: deleting folder %s: %w", opts.Path, err)
		}
		return nil
	}
	logToSession(
		fmt.Sprintf("Deleting folder [%s] without recursion...", opts.Path),
		LogSeverityInfo, "Remove-ADTFolder",
	)
	return removeFolderNonRecursive(opts.Path)
}

// removeFolderNonRecursive deletes the folder's files and empty subfolders,
// then the folder itself; non-empty subfolders abort with errFolderNotEmpty.
func removeFolderNonRecursive(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		return fmt.Errorf("adt: reading folder %s: %w", path, err)
	}
	var skipped []string
	for _, entry := range entries {
		child := filepath.Join(path, entry.Name())
		if entry.IsDir() {
			if err := os.Remove(child); err != nil { // only succeeds when empty
				skipped = append(skipped, entry.Name())
			}
			continue
		}
		if err := os.Remove(child); err != nil {
			return fmt.Errorf("adt: deleting %s: %w", child, err)
		}
	}
	if len(skipped) > 0 {
		return fmt.Errorf("adt: the following subfolders are not empty ['%s']: %w",
			strings.Join(skipped, "'; '"), errFolderNotEmpty)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("adt: deleting folder %s: %w", path, err)
	}
	return nil
}

// UserProfileFilterOptions selects which profiles the *-ADTFile*UserProfiles
// functions touch; it is Get-ADTUserProfiles' parameter set (the zero value
// excludes system and service profiles and keeps the Default User template,
// matching PSADT's defaults).
type UserProfileFilterOptions = GetADTUserProfilesOptions

// getUserProfilePaths is the package seam for user-profile enumeration: it
// returns the ProfilePath of every profile matching the filter. The default
// delegates to GetADTUserProfiles (adt/users.go); it is a variable so
// tests can substitute fixed profile roots.
var getUserProfilePaths = func(ctx context.Context, opts UserProfileFilterOptions) ([]string, error) {
	profiles, err := GetADTUserProfiles(ctx, opts)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(profiles))
	for _, p := range profiles {
		if p.ProfilePath != "" {
			paths = append(paths, p.ProfilePath)
		}
	}
	return paths, nil
}

// CopyADTFileToUserProfilesOptions mirrors Copy-ADTFileToUserProfiles.
type CopyADTFileToUserProfilesOptions struct {
	// Path lists the files or folders to copy; glob patterns supported.
	Path []string
	// Destination is the path relative to each profile root (for example
	// `AppData\Roaming\Vendor`). Empty copies to the profile root.
	Destination string
	// Recurse copies files in subdirectories.
	Recurse bool
	// Flatten copies all files into each profile's destination root.
	Flatten bool
	// ContinueFileCopyOnError continues after per-file failures.
	ContinueFileCopyOnError bool
	// FileCopyMode is "Native" or "Robocopy" ("" = session config default).
	FileCopyMode string
	// Profiles filters which user profiles receive the copy.
	Profiles UserProfileFilterOptions
}

// CopyADTFileToUserProfiles is the Go port of Copy-ADTFileToUserProfiles: it
// copies the given paths into each matching user profile.
func CopyADTFileToUserProfiles(ctx context.Context, opts CopyADTFileToUserProfilesOptions) error {
	profiles, err := getUserProfilePaths(ctx, opts.Profiles)
	if err != nil {
		return err
	}
	var errs []error
	for _, profile := range profiles {
		dest := profile
		if opts.Destination != "" {
			dest = filepath.Join(profile, opts.Destination)
		}
		logToSession(
			fmt.Sprintf("Copying path [%s] to %s.", strings.Join(opts.Path, ", "), dest),
			LogSeverityInfo, "Copy-ADTFileToUserProfiles",
		)
		err := CopyADTFile(ctx, CopyADTFileOptions{
			Path:                    opts.Path,
			Destination:             dest,
			Recurse:                 opts.Recurse,
			Flatten:                 opts.Flatten,
			ContinueFileCopyOnError: opts.ContinueFileCopyOnError,
			FileCopyMode:            opts.FileCopyMode,
		})
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// RemoveADTFileFromUserProfilesOptions mirrors Remove-ADTFileFromUserProfiles.
type RemoveADTFileFromUserProfilesOptions struct {
	// Path lists profile-relative paths to remove (glob patterns
	// supported); missing paths are logged and skipped.
	Path []string
	// Recurse deletes folders recursively.
	Recurse bool
	// Profiles filters which user profiles are processed.
	Profiles UserProfileFilterOptions
}

// RemoveADTFileFromUserProfiles is the Go port of
// Remove-ADTFileFromUserProfiles: it removes the given profile-relative
// paths from each matching user profile.
func RemoveADTFileFromUserProfiles(ctx context.Context, opts RemoveADTFileFromUserProfilesOptions) error {
	profiles, err := getUserProfilePaths(ctx, opts.Profiles)
	if err != nil {
		return err
	}
	var errs []error
	for _, profile := range profiles {
		paths := make([]string, 0, len(opts.Path))
		for _, p := range opts.Path {
			paths = append(paths, filepath.Join(profile, p))
		}
		logToSession(
			fmt.Sprintf("Removing path [%s] from %s.", strings.Join(opts.Path, ", "), profile),
			LogSeverityInfo, "Remove-ADTFileFromUserProfiles",
		)
		if err := RemoveADTFile(ctx, RemoveADTFileOptions{Path: paths, Recurse: opts.Recurse}); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// CopyADTContentToCache is the Go port of Copy-ADTContentToCache: it copies
// the active session's toolkit content (its ScriptDirectory, including Files
// and SupportFiles) to the cache folder and returns the cache path. An empty
// path uses the default `<Toolkit.CachePath>\<InstallName>`.
func CopyADTContentToCache(ctx context.Context, path string) (string, error) {
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	scriptDir := s.Options().ScriptDirectory
	if scriptDir == "" {
		return "", winerr.Wrap(
			"CopyADTContentToCache: the active session has no ScriptDirectory established",
			winerr.ErrInvalidOption,
		)
	}
	if fi, err := os.Stat(scriptDir); err != nil || !fi.IsDir() {
		return "", fmt.Errorf("adt: script directory %s: %w", scriptDir, winerr.ErrNotFound)
	}
	cachePath := path
	if cachePath == "" {
		cachePath = filepath.Join(s.Config().Toolkit.CachePath, s.InstallName())
	}
	if sameFilesystemPath(scriptDir, cachePath) {
		logToSession(
			fmt.Sprintf("Source and destination are the same path [%s]. Skipping copy operation.", cachePath),
			LogSeverityInfo, "Copy-ADTContentToCache",
		)
		return cachePath, nil
	}
	logToSession(
		fmt.Sprintf("Copying toolkit content to cache folder [%s].", cachePath),
		LogSeverityInfo, "Copy-ADTContentToCache",
	)
	if err := os.MkdirAll(cachePath, 0o755); err != nil { //#nosec G301 -- parity with New-Item defaults
		return "", fmt.Errorf("adt: creating cache folder %s: %w", cachePath, err)
	}
	err = fsops.Copy(ctx, []string{filepath.Join(scriptDir, "*")}, cachePath, fsops.CopyOptions{
		Recurse: true,
		Mode:    fsops.ModeNative,
	})
	if err != nil {
		return "", err
	}
	return cachePath, nil
}

// sameFilesystemPath reports whether two paths resolve to the same location.
func sameFilesystemPath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

// RemoveADTContentFromCache is the Go port of Remove-ADTContentFromCache: it
// removes the active session's cache folder
// (`<Toolkit.CachePath>\<InstallName>`), tolerating a missing folder.
func RemoveADTContentFromCache(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := GetADTSession()
	if err != nil {
		return err
	}
	cachePath := filepath.Join(s.Config().Toolkit.CachePath, s.InstallName())
	if fi, err := os.Stat(cachePath); err != nil || !fi.IsDir() {
		logToSession(
			fmt.Sprintf("Cache folder [%s] does not exist.", cachePath),
			LogSeverityInfo, "Remove-ADTContentFromCache",
		)
		return nil
	}
	logToSession(
		fmt.Sprintf("Removing cache folder [%s].", cachePath),
		LogSeverityInfo, "Remove-ADTContentFromCache",
	)
	if err := os.RemoveAll(cachePath); err != nil {
		return fmt.Errorf("adt: removing cache folder %s: %w", cachePath, err)
	}
	return nil
}

// NewADTZipFileOptions mirrors the parameters of New-ADTZipFile.
type NewADTZipFileOptions struct {
	// LiteralPath lists the files or folders to archive.
	LiteralPath []string
	// DestinationPath is the .zip file to create.
	DestinationPath string
	// RemoveSourceAfterArchiving recursively deletes each source after the
	// archive has been created successfully.
	RemoveSourceAfterArchiving bool
	// Overwrite deletes an existing archive first (Force in PSADT).
	Overwrite bool
}

// NewADTZipFile is the Go port of New-ADTZipFile: it creates a zip archive
// from the given sources using the standard library archiver.
func NewADTZipFile(ctx context.Context, opts NewADTZipFileOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	if len(opts.LiteralPath) == 0 || strings.TrimSpace(opts.DestinationPath) == "" {
		return winerr.Wrap("NewADTZipFile: LiteralPath and DestinationPath are required",
			winerr.ErrInvalidOption)
	}
	if _, err := os.Stat(opts.DestinationPath); err == nil {
		if !opts.Overwrite {
			return fmt.Errorf("adt: archive %s already exists: %w", opts.DestinationPath, fs.ErrExist)
		}
		logToSession(
			fmt.Sprintf("An archive at the destination path already exists, deleting file [%s].",
				opts.DestinationPath),
			LogSeverityInfo, "New-ADTZipFile",
		)
		if err := os.Remove(opts.DestinationPath); err != nil {
			return fmt.Errorf("adt: deleting existing archive %s: %w", opts.DestinationPath, err)
		}
	}
	logToSession(
		fmt.Sprintf("Compressing [%s] to destination path [%s]...",
			strings.Join(opts.LiteralPath, ", "), opts.DestinationPath),
		LogSeverityInfo, "New-ADTZipFile",
	)
	if err := archive.WriteZipArchive(opts.LiteralPath, opts.DestinationPath); err != nil {
		return err
	}
	if opts.RemoveSourceAfterArchiving {
		for _, src := range opts.LiteralPath {
			logToSession(
				fmt.Sprintf("Recursively deleting [%s] as contents have been successfully archived.", src),
				LogSeverityInfo, "New-ADTZipFile",
			)
			if err := os.RemoveAll(src); err != nil {
				return fmt.Errorf("adt: deleting archived source %s: %w", src, err)
			}
		}
	}
	return nil
}

// GetADTFileVersion is the Go port of Get-ADTFileVersion: it returns the
// file's binary version string ("a.b.c.d"). Windows-only.
func GetADTFileVersion(ctx context.Context, path string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("adt: %w", err)
	}
	info, err := fsops.GetFileVersion(path)
	if err != nil {
		return "", err
	}
	logToSession(
		fmt.Sprintf("File version is [%s].", info.FileVersion),
		LogSeverityInfo, "Get-ADTFileVersion",
	)
	return info.FileVersion, nil
}

// GetADTFileVersionInfo returns both the file version and product version of
// the file's version resource (see fsops.FileVersionInfo). Windows-only.
func GetADTFileVersionInfo(ctx context.Context, path string) (fsops.FileVersionInfo, error) {
	if err := ctx.Err(); err != nil {
		return fsops.FileVersionInfo{}, fmt.Errorf("adt: %w", err)
	}
	return fsops.GetFileVersion(path)
}

// GetADTFreeDiskSpace is the Go port of Get-ADTFreeDiskSpace: it returns the
// free megabytes on the given drive or path (empty defaults to the system
// drive). Windows-only.
func GetADTFreeDiskSpace(ctx context.Context, drive string) (uint64, error) {
	if err := ctx.Err(); err != nil {
		return 0, fmt.Errorf("adt: %w", err)
	}
	if drive == "" {
		if sysDrive := os.Getenv("SystemDrive"); sysDrive != "" {
			drive = sysDrive + `\`
		}
	}
	logToSession(
		fmt.Sprintf("Retrieving free disk space for drive [%s].", drive),
		LogSeverityInfo, "Get-ADTFreeDiskSpace",
	)
	free, err := fsops.FreeDiskSpaceMB(drive)
	if err != nil {
		return 0, err
	}
	logToSession(
		fmt.Sprintf("Free disk space for drive [%s]: [%d MB].", drive, free),
		LogSeverityInfo, "Get-ADTFreeDiskSpace",
	)
	return free, nil
}

// invalidFileNameChars matches the characters reported by
// System.IO.Path.GetInvalidFileNameChars on Windows.
var invalidFileNameChars = regexp.MustCompile(`[\x00-\x1f<>:"/\\|?*]`)

// RemoveADTInvalidFileNameChars is the Go port of
// Remove-ADTInvalidFileNameChars: it strips invalid file name characters
// from the given string and trims surrounding whitespace.
func RemoveADTInvalidFileNameChars(name string) string {
	return strings.TrimSpace(invalidFileNameChars.ReplaceAllString(name, ""))
}

// SetADTItemPermissionOptions mirrors the parameters of
// Set-ADTItemPermission (see fsops.ItemPermissionOptions for field docs).
type SetADTItemPermissionOptions = fsops.ItemPermissionOptions

// SetADTItemPermission is the Go port of Set-ADTItemPermission: it applies
// grant/deny/remove ACL changes and inheritance toggles to a file, folder or
// registry key. Windows-only.
func SetADTItemPermission(ctx context.Context, opts SetADTItemPermissionOptions) error {
	logToSession(
		fmt.Sprintf("Changing permissions [%s:%s] on path [%s] for user [%s].",
			opts.Action, opts.Permission, opts.Path, opts.User),
		LogSeverityInfo, "Set-ADTItemPermission",
	)
	return fsops.SetItemPermission(ctx, opts)
}
