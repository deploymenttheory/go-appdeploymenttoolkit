package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// WriteZipArchive creates a zip archive at dest from the given sources: files
// are stored under their base names, directories as trees rooted at their
// base names. A failed write removes the partial archive.
func WriteZipArchive(sources []string, dest string) error {
	//#nosec G304 -- caller-supplied deployment paths by design
	out, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("fsops: creating archive %s: %w", dest, err)
	}
	zw := zip.NewWriter(out)
	for _, src := range sources {
		if err := addZipEntry(zw, src); err != nil {
			_ = zw.Close()
			_ = out.Close()
			_ = os.Remove(dest)
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = out.Close()
		return fmt.Errorf("fsops: finalizing archive %s: %w", dest, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("fsops: closing archive %s: %w", dest, err)
	}
	return nil
}

// addZipEntry adds a file, or a folder tree rooted at its base name, to zw.
func addZipEntry(zw *zip.Writer, src string) error {
	fi, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("fsops: archive source %s: %w", src, errs.ErrNotFound)
	}
	if !fi.IsDir() {
		return addZipFile(zw, src, filepath.Base(src), fi)
	}
	base := filepath.Base(src)
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("fsops: walking %s: %w", path, err)
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("fsops: relativizing %s: %w", path, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("fsops: reading %s: %w", path, err)
		}
		return addZipFile(zw, path, filepath.ToSlash(filepath.Join(base, rel)), info)
	})
}

// addZipFile streams one file into the archive, preserving its mod time.
func addZipFile(zw *zip.Writer, path, name string, fi fs.FileInfo) error {
	hdr, err := zip.FileInfoHeader(fi)
	if err != nil {
		return fmt.Errorf("fsops: archiving %s: %w", path, err)
	}
	hdr.Name = name
	hdr.Method = zip.Deflate
	w, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("fsops: archiving %s: %w", path, err)
	}
	in, err := os.Open(path) //#nosec G304 -- caller-supplied deployment paths by design
	if err != nil {
		return fmt.Errorf("fsops: opening %s: %w", path, err)
	}
	defer in.Close() //nolint:errcheck // read-only handle
	if _, err := io.Copy(w, in); err != nil {
		return fmt.Errorf("fsops: archiving %s: %w", path, err)
	}
	return nil
}
