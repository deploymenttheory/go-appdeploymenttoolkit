// Package fsops implements the filesystem operations behind PSADT's
// file-management functions (Copy-ADTFile and friends): recursive and
// flattened copies, robocopy delegation on Windows, file version queries,
// free disk space and NTFS permission management.
//
// The copy engine is deliberately portable so its PSADT semantics are
// unit-testable on every platform; Windows-only features live in *_windows.go
// files with stubs returning winerr.ErrNotWindows elsewhere.
package fsops

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Mode selects the copy engine, mirroring config.psd1 Toolkit.FileCopyMode.
type Mode string

// Mode values.
const (
	ModeNative   Mode = "Native"
	ModeRobocopy Mode = "Robocopy"
)

// ParseMode parses the PSADT string form (case-insensitive); the empty
// string defaults to Native.
func ParseMode(s string) (Mode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "native":
		return ModeNative, nil
	case "robocopy":
		return ModeRobocopy, nil
	default:
		return "", fmt.Errorf("fsops: unknown FileCopyMode %q: %w", s, winerr.ErrInvalidOption)
	}
}

// ErrRobocopyFailed reports a robocopy run that ended with exit code >= 8.
var ErrRobocopyFailed = errors.New("fsops: robocopy failed")

// CopyOptions mirrors the switches of Copy-ADTFile.
type CopyOptions struct {
	// Recurse copies files in subdirectories.
	Recurse bool
	// Flatten copies every file from all subtrees into the destination root.
	Flatten bool
	// ContinueOnError collects per-file errors and keeps copying; the
	// joined error is returned at the end.
	ContinueOnError bool
	// Mode selects the copy engine (Native default; Robocopy is
	// Windows-only and falls back to Native when unusable).
	Mode Mode
}

// Copy is the engine behind Copy-ADTFile: it copies each path (glob patterns
// supported) to dest with PSADT's destination-folder inference, preserving
// file modes and modification times.
func Copy(ctx context.Context, paths []string, dest string, opts CopyOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("fsops: %w", err)
	}
	mode, err := ParseMode(string(opts.Mode))
	if err != nil {
		return err
	}
	if len(paths) == 0 || strings.TrimSpace(dest) == "" {
		return fmt.Errorf("fsops: copy requires at least one source path and a destination: %w",
			winerr.ErrInvalidOption)
	}
	if mode == ModeRobocopy {
		return copyRobocopy(ctx, paths, dest, opts)
	}
	return copyNative(ctx, paths, dest, opts)
}

// copyNative ports Copy-ADTFile's Native (Copy-Item) branch.
func copyNative(ctx context.Context, paths []string, dest string, opts CopyOptions) error {
	var errs []error
	// fail either aborts (default) or records the error and continues
	// (ContinueFileCopyOnError semantics).
	fail := func(err error) error {
		if err == nil {
			return nil
		}
		if opts.ContinueOnError {
			errs = append(errs, err)
			return nil
		}
		return err
	}
	for _, pattern := range paths {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("fsops: %w", err)
		}
		sources, err := expandSourcePath(pattern)
		if err != nil {
			if abort := fail(err); abort != nil {
				return abort
			}
			continue
		}
		if err := ensureDestination(dest); err != nil {
			return err
		}
		for _, src := range sources {
			if abort := fail(copyOne(src, dest, opts, fail)); abort != nil {
				return abort
			}
		}
	}
	return errors.Join(errs...)
}

// expandSourcePath resolves a source path or glob pattern to concrete paths,
// erroring (winerr.ErrNotFound) when nothing exists, like Get-Item.
func expandSourcePath(pattern string) ([]string, error) {
	if strings.ContainsAny(pattern, "*?[") {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("fsops: invalid source pattern %s: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("fsops: source path %s: %w", pattern, winerr.ErrNotFound)
		}
		return matches, nil
	}
	if _, err := os.Stat(pattern); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("fsops: source path %s: %w", pattern, winerr.ErrNotFound)
		}
		return nil, fmt.Errorf("fsops: source path %s: %w", pattern, err)
	}
	return []string{pattern}, nil
}

// destinationIsContainer ports PSADT's inference: an existing directory, or a
// path whose leaf has no extension (or an extension with no base name, such
// as ".config") is treated as a folder.
func destinationIsContainer(dest string) bool {
	if fi, err := os.Stat(dest); err == nil {
		return fi.IsDir()
	}
	base := filepath.Base(dest)
	ext := filepath.Ext(base)
	return ext == "" || strings.TrimSuffix(base, ext) == ""
}

// ensureDestination pre-creates the destination folder (or the parent folder
// of a file destination), mirroring Copy-ADTFile.
func ensureDestination(dest string) error {
	dir := dest
	if !destinationIsContainer(dest) {
		dir = filepath.Dir(dest)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil { //#nosec G301 -- parity with New-Item defaults
		return fmt.Errorf("fsops: creating destination %s: %w", dir, err)
	}
	return nil
}

// copyOne dispatches a single resolved source path.
func copyOne(src, dest string, opts CopyOptions, fail func(error) error) error {
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("fsops: source path %s: %w", src, err)
	}
	switch {
	case opts.Flatten:
		return copyFlattened(src, fi, dest, fail)
	case fi.IsDir():
		target := filepath.Join(dest, filepath.Base(src))
		if !opts.Recurse {
			// Without -Recurse, Copy-Item materializes just the folder.
			if err := os.MkdirAll(target, fi.Mode().Perm()); err != nil {
				return fmt.Errorf("fsops: creating folder %s: %w", target, err)
			}
			return nil
		}
		return copyTree(src, target, fail)
	case destinationIsContainer(dest):
		return copyFile(src, filepath.Join(dest, filepath.Base(src)))
	default:
		return copyFile(src, dest)
	}
}

// copyFlattened copies every file beneath src (or src itself when it is a
// file) into the destination root.
func copyFlattened(src string, fi fs.FileInfo, dest string, fail func(error) error) error {
	if !fi.IsDir() {
		return copyFile(src, filepath.Join(dest, filepath.Base(src)))
	}
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fail(fmt.Errorf("fsops: walking %s: %w", path, err))
		}
		if d.IsDir() {
			return nil
		}
		return fail(copyFile(path, filepath.Join(dest, d.Name())))
	})
}

// copyTree recursively copies the directory src to dst, preserving relative
// structure, file modes and modification times.
func copyTree(src, dst string, fail func(error) error) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fail(fmt.Errorf("fsops: walking %s: %w", path, err))
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("fsops: relativizing %s: %w", path, err)
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return fail(fmt.Errorf("fsops: reading %s: %w", path, err))
			}
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("fsops: creating folder %s: %w", target, err)
			}
			return nil
		}
		return fail(copyFile(path, target))
	})
}

// copyFile copies a single file, preserving its mode and modification time.
func copyFile(src, dst string) error {
	in, err := os.Open(src) //#nosec G304 -- caller-supplied deployment paths by design
	if err != nil {
		return fmt.Errorf("fsops: opening %s: %w", src, err)
	}
	defer in.Close() //nolint:errcheck // read-only handle
	fi, err := in.Stat()
	if err != nil {
		return fmt.Errorf("fsops: reading %s: %w", src, err)
	}
	//#nosec G304 -- caller-supplied deployment paths by design
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode().Perm())
	if err != nil {
		return fmt.Errorf("fsops: creating %s: %w", dst, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("fsops: copying %s to %s: %w", src, dst, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("fsops: closing %s: %w", dst, err)
	}
	if err := os.Chtimes(dst, fi.ModTime(), fi.ModTime()); err != nil {
		return fmt.Errorf("fsops: preserving times on %s: %w", dst, err)
	}
	return nil
}
