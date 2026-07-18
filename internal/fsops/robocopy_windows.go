package fsops

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// robocopyDefaultParams mirrors Copy-ADTFile's default $RobocopyParams.
var robocopyDefaultParams = []string{
	"/NJH", "/NJS", "/NS", "/NC", "/NP", "/NDL", "/FP",
	"/IA:RASHCNETO", "/IS", "/IT", "/IM", "/XX", "/MT:4", "/R:1", "/W:1",
}

// wildcardInFolder matches an asterisk in the folder portion of a path,
// which robocopy cannot handle (Copy-ADTFile falls back to Native).
var wildcardInFolder = regexp.MustCompile(`\*.*[\\/]`)

// copyRobocopy ports Copy-ADTFile's Robocopy branch: it shells out to
// robocopy.exe with PSADT's default arguments and falls back to the native
// engine whenever robocopy cannot service the request.
func copyRobocopy(ctx context.Context, paths []string, dest string, opts CopyOptions) error {
	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "Robocopy.exe")
	//#nosec G703 -- SystemRoot is a trusted, system-managed variable
	if _, err := os.Stat(exe); err != nil {
		return copyNative(ctx, paths, dest, opts) // robocopy unavailable
	}
	for _, p := range paths {
		if wildcardInFolder.MatchString(p) {
			return copyNative(ctx, paths, dest, opts)
		}
	}
	if !destinationIsContainer(dest) {
		return copyNative(ctx, paths, dest, opts) // destination appears to be a file
	}

	params := append([]string(nil), robocopyDefaultParams...)
	if opts.Recurse && !opts.Flatten {
		params = append(params, "/E")
	}

	var errs []error
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
	for _, srcPath := range paths {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("fsops: %w", err)
		}
		if abort := fail(robocopyOne(ctx, exe, srcPath, dest, params, opts, fail)); abort != nil {
			return abort
		}
	}
	return errors.Join(errs...)
}

// robocopyOne copies a single source path (file, folder or leaf wildcard).
func robocopyOne(
	ctx context.Context,
	exe, srcPath, dest string,
	params []string,
	opts CopyOptions,
	fail func(error) error,
) error {
	if err := os.MkdirAll(dest, 0o755); err != nil { //#nosec G301 -- parity with New-Item defaults
		return fmt.Errorf("fsops: creating destination %s: %w", dest, err)
	}

	// Split the source into <folder> <file-pattern> like Copy-ADTFile.
	srcDir, filePattern, destDir := robocopyTriple(srcPath, dest)

	if opts.Flatten {
		// Copy matching files from the source root and every subfolder
		// into the destination root, non-recursively per folder.
		if err := fail(runRobocopy(ctx, exe, srcDir, dest, filePattern, params)); err != nil {
			return err
		}
		return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return fail(fmt.Errorf("fsops: walking %s: %w", path, err))
			}
			if !d.IsDir() || path == srcDir {
				return nil
			}
			return fail(runRobocopy(ctx, exe, path, dest, filePattern, params))
		})
	}
	return runRobocopy(ctx, exe, srcDir, destDir, filePattern, params)
}

// robocopyTriple resolves the <source-folder> <file> <destination-folder>
// argument set. Folder sources copy into dest\<basename> so robocopy matches
// native PowerShell results.
func robocopyTriple(srcPath, dest string) (srcDir, filePattern, destDir string) {
	trimmed := strings.TrimRight(srcPath, `\/`)
	if fi, err := os.Stat(trimmed); err == nil && fi.IsDir() {
		return trimmed, "*", filepath.Join(dest, filepath.Base(trimmed))
	}
	return filepath.Dir(trimmed), filepath.Base(trimmed), dest
}

// runRobocopy executes one robocopy pass; exit codes below 8 are success.
func runRobocopy(
	ctx context.Context,
	exe, srcDir, destDir, filePattern string,
	params []string,
) error {
	args := append([]string{srcDir, destDir, filePattern}, params...)
	//#nosec G204 G702 -- fixed system32 robocopy path; args are deployment inputs by design
	cmd := exec.CommandContext(ctx, exe, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			return fmt.Errorf("fsops: executing robocopy: %w", err)
		}
	}
	code := cmd.ProcessState.ExitCode()
	if code >= 8 {
		return fmt.Errorf("fsops: robocopy %s -> %s exit code %d: %s: %w",
			srcDir, destDir, code, strings.TrimSpace(string(out)), ErrRobocopyFailed)
	}
	return nil
}
