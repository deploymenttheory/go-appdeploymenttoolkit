package winadt

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// InstallADTMSUpdatesOptions mirrors Install-ADTMSUpdates.
type InstallADTMSUpdatesOptions struct {
	// Directory holds the .msu updates to install (searched recursively), or
	// a single .msu file.
	Directory string
}

// InstallADTMSUpdates is the Go port of Install-ADTMSUpdates: it installs
// every Windows Update package (.msu) found under the directory (recursively)
// with `wusa.exe`-style quiet, no-restart semantics via StartADTProcess.
func InstallADTMSUpdates(ctx context.Context, opts InstallADTMSUpdatesOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	updates, err := discoverMSUpdates(opts.Directory)
	if err != nil {
		return err
	}
	if len(updates) == 0 {
		return fmt.Errorf("adt: no .msu updates found in %s: %w", opts.Directory, winerr.ErrNotFound)
	}
	logSessionInfo(fmt.Sprintf("Installing %d Microsoft update(s) from [%s].", len(updates), opts.Directory), "InstallADTMSUpdates")
	for _, update := range updates {
		if _, err := StartADTProcess(ctx, StartADTProcessOptions{
			FilePath:     update,
			ArgumentList: "/quiet /norestart",
			WindowStyle:  "Hidden",
		}); err != nil {
			return err
		}
	}
	return nil
}

// discoverMSUpdates returns the .msu files at path: a single file when path is
// an .msu, or every .msu beneath path when it is a directory.
func discoverMSUpdates(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("adt: reading update path: %w", err)
	}
	if !info.IsDir() {
		if !strings.EqualFold(filepath.Ext(path), ".msu") {
			return nil, winerr.Wrap("adt: update path is not a .msu file", winerr.ErrInvalidOption)
		}
		return []string{path}, nil
	}
	var updates []string
	err = filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".msu") {
			updates = append(updates, p)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("adt: scanning for updates: %w", err)
	}
	return updates, nil
}
