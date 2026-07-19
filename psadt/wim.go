package psadt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// MountedWimFile describes a mounted WIM image, mirroring the salient fields of
// PSADT's returned Microsoft.Dism.Commands.ImageObject.
type MountedWimFile struct {
	// ImagePath is the source WIM file.
	ImagePath string
	// Path is the directory the image is mounted to.
	Path string
	// Index is the mounted image index (0 when mounted by name).
	Index int
	// Name is the mounted image name (empty when mounted by index).
	Name string
}

// MountADTWimFileOptions mirrors the parameters of Mount-ADTWimFile.
type MountADTWimFileOptions struct {
	// ImagePath is the WIM file to mount (must not be a network share).
	ImagePath string
	// Path is the directory to mount the image to.
	Path string
	// Index selects the image by index (mutually exclusive with Name).
	Index int
	// Name selects the image by name (mutually exclusive with Index).
	Name string
	// Force removes a non-empty mount directory before mounting.
	Force bool
}

// DismountADTWimFileOptions mirrors the parameters of Dismount-ADTWimFile.
type DismountADTWimFileOptions struct {
	// Path is the WIM mount directory to dismount.
	Path string
	// Save commits changes on dismount; the default discards them (PSADT
	// always discards).
	Save bool
}

// MountADTWimFile is the Go port of Mount-ADTWimFile: it mounts an image from a
// WIM file to a directory using dism.exe.
//
// Implementation note: go-bindings-win32 exposes neither WIMGAPI nor DISMAPI,
// and PSADT itself drives the DISM PowerShell cmdlets (Mount-WindowsImage), so
// this port shells out to %WINDIR%\System32\dism.exe via StartADTProcess. As
// in PSADT the image is mounted read-only with integrity checking.
func MountADTWimFile(ctx context.Context, opts MountADTWimFileOptions) (*MountedWimFile, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("psadt: MountADTWimFile: %w", err)
	}
	if err := validateMountOptions(opts); err != nil {
		return nil, err
	}
	if info, err := os.Stat(opts.ImagePath); err != nil || info.IsDir() {
		return nil, fmt.Errorf("psadt: image path [%s] cannot be found: %w", opts.ImagePath, ErrNotFound)
	}

	logToSession(fmt.Sprintf("Mounting WIM file [%s] to [%s].", opts.ImagePath, opts.Path),
		LogSeverityInfo, "MountADTWimFile")

	if err := prepareMountDir(opts.Path, opts.Force); err != nil {
		return nil, err
	}

	if _, err := StartADTProcess(ctx, StartADTProcessOptions{
		FilePath:       dismExePath(),
		ArgumentList:   buildDismMountArgs(opts),
		CreateNoWindow: true,
	}); err != nil {
		return nil, err
	}
	logToSession(fmt.Sprintf("Successfully mounted WIM file [%s].", opts.ImagePath),
		LogSeveritySuccess, "MountADTWimFile")
	return &MountedWimFile{
		ImagePath: opts.ImagePath,
		Path:      opts.Path,
		Index:     opts.Index,
		Name:      opts.Name,
	}, nil
}

// DismountADTWimFile is the Go port of Dismount-ADTWimFile: it dismounts a WIM
// image (discarding changes by default) via dism.exe and removes the now-empty
// mount directory.
func DismountADTWimFile(ctx context.Context, opts DismountADTWimFileOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: DismountADTWimFile: %w", err)
	}
	if strings.TrimSpace(opts.Path) == "" {
		return fmt.Errorf("psadt: Path is required: %w", ErrInvalidOption)
	}
	logToSession(fmt.Sprintf("Dismounting WIM file at path [%s].", opts.Path),
		LogSeverityInfo, "DismountADTWimFile")

	if _, err := StartADTProcess(ctx, StartADTProcessOptions{
		FilePath:       dismExePath(),
		ArgumentList:   buildDismUnmountArgs(opts.Path, opts.Save),
		CreateNoWindow: true,
	}); err != nil {
		return err
	}
	logToSession("Successfully dismounted WIM file.", LogSeveritySuccess, "DismountADTWimFile")
	if err := os.Remove(opts.Path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("psadt: removing mount directory: %w", err)
	}
	return nil
}

// validateMountOptions enforces Mount-ADTWimFile's parameter sets: ImagePath
// and Path are required, and exactly one of Index or Name must be supplied.
func validateMountOptions(opts MountADTWimFileOptions) error {
	if strings.TrimSpace(opts.ImagePath) == "" {
		return fmt.Errorf("psadt: ImagePath is required: %w", ErrInvalidOption)
	}
	if strings.TrimSpace(opts.Path) == "" {
		return fmt.Errorf("psadt: Path is required: %w", ErrInvalidOption)
	}
	hasName := strings.TrimSpace(opts.Name) != ""
	switch {
	case opts.Index > 0 && hasName:
		return fmt.Errorf("psadt: specify either Index or Name, not both: %w", ErrInvalidOption)
	case opts.Index <= 0 && !hasName:
		return fmt.Errorf("psadt: an image Index or Name is required: %w", ErrInvalidOption)
	case opts.Index < 0:
		return fmt.Errorf("psadt: Index must be a positive image index: %w", ErrInvalidOption)
	}
	return nil
}

// prepareMountDir ports Mount-ADTWimFile's directory handling: a non-empty
// mount directory is an error unless Force is set (in which case it is
// removed), and the directory is created when absent.
func prepareMountDir(path string, force bool) error {
	entries, err := os.ReadDir(path)
	switch {
	case err == nil && len(entries) > 0:
		if !force {
			return fmt.Errorf("psadt: mount path [%s] is not empty: %w", path, ErrInvalidOption)
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("psadt: removing pre-existing mount path: %w", err)
		}
	case err != nil && !os.IsNotExist(err):
		return fmt.Errorf("psadt: inspecting mount path: %w", err)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("psadt: creating mount path: %w", err)
	}
	return nil
}

// buildDismMountArgs composes the dism.exe argument string for mounting an
// image, mirroring PSADT's read-only, integrity-checked Mount-WindowsImage.
func buildDismMountArgs(opts MountADTWimFileOptions) string {
	parts := []string{
		"/Mount-Wim",
		`/WimFile:"` + opts.ImagePath + `"`,
		`/MountDir:"` + opts.Path + `"`,
	}
	if strings.TrimSpace(opts.Name) != "" {
		parts = append(parts, `/Name:"`+opts.Name+`"`)
	} else {
		parts = append(parts, "/Index:"+strconv.Itoa(opts.Index))
	}
	parts = append(parts, "/ReadOnly", "/CheckIntegrity")
	return strings.Join(parts, " ")
}

// buildDismUnmountArgs composes the dism.exe argument string for dismounting an
// image; changes are committed when save is set, otherwise discarded.
func buildDismUnmountArgs(path string, save bool) string {
	commit := "/Discard"
	if save {
		commit = "/Commit"
	}
	return strings.Join([]string{"/Unmount-Wim", `/MountDir:"` + path + `"`, commit}, " ")
}

// dismExePath returns %WINDIR%\System32\dism.exe, matching PSADT's use of the
// system directory.
func dismExePath() string {
	if windir := windowsDir(); windir != "" {
		return filepath.Join(windir, "System32", "dism.exe")
	}
	return "dism.exe"
}
