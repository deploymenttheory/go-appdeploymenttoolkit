package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Options configures a session log writer.
type Options struct {
	Directory  string // log directory, created if missing
	FileName   string // e.g. "Contoso_App_1.0_EN_01_PSAppDeployToolkit_Install.log"
	Style      Style
	Append     bool // config Toolkit.LogAppend
	MaxSizeMB  int  // config Toolkit.LogMaxSize; 0 disables rotation
	MaxHistory int  // config Toolkit.LogMaxHistory
	// Echo receives each rendered line (e.g. console/zap mirror). Optional.
	Echo func(e Entry)
}

// Writer appends formatted entries to the session log file, rotating when the
// file exceeds MaxSizeMB. Safe for concurrent use.
type Writer struct {
	mu   sync.Mutex
	opts Options
	file *os.File
	path string
}

// NewWriter opens (or truncates, when Append is false) the session log file.
func NewWriter(opts Options) (*Writer, error) {
	if err := os.MkdirAll(opts.Directory, 0o755); err != nil {
		return nil, fmt.Errorf("logging: creating log directory: %w", err)
	}
	w := &Writer{opts: opts, path: filepath.Join(opts.Directory, opts.FileName)}
	if err := w.open(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *Writer) open() error {
	flags := os.O_CREATE | os.O_WRONLY
	if w.opts.Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}
	f, err := os.OpenFile(w.path, flags, 0o644)
	if err != nil {
		return fmt.Errorf("logging: opening log file: %w", err)
	}
	w.file = f
	return nil
}

// Path returns the full path of the active log file.
func (w *Writer) Path() string {
	return w.path
}

// Write renders and appends the entry, then echoes it if configured.
func (w *Writer) Write(e Entry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return fmt.Errorf("logging: %w", os.ErrClosed)
	}
	if err := w.rotateLocked(); err != nil {
		return err
	}
	if _, err := w.file.WriteString(e.Line(w.opts.Style) + "\r\n"); err != nil {
		return fmt.Errorf("logging: writing entry: %w", err)
	}
	if w.opts.Echo != nil {
		w.opts.Echo(e)
	}
	return nil
}

// rotateLocked archives the current file as <base>_<timestamp>.log when it
// exceeds the size limit and prunes archives beyond MaxHistory.
func (w *Writer) rotateLocked() error {
	if w.opts.MaxSizeMB <= 0 {
		return nil
	}
	info, err := w.file.Stat()
	if err != nil || info.Size() < int64(w.opts.MaxSizeMB)*1024*1024 {
		return nil //nolint:nilerr // an unstattable file simply skips rotation
	}
	if err := w.file.Close(); err != nil {
		return fmt.Errorf("logging: closing log for rotation: %w", err)
	}
	base := strings.TrimSuffix(w.opts.FileName, filepath.Ext(w.opts.FileName))
	archive := filepath.Join(
		w.opts.Directory,
		fmt.Sprintf(
			"%s_%s%s",
			base,
			time.Now().Format("20060102150405"),
			filepath.Ext(w.opts.FileName),
		),
	)
	if err := os.Rename(w.path, archive); err != nil {
		return fmt.Errorf("logging: archiving log: %w", err)
	}
	w.pruneArchives(base)
	// Reopen fresh (never append to a just-rotated file).
	appendPrev := w.opts.Append
	w.opts.Append = false
	err = w.open()
	w.opts.Append = appendPrev
	return err
}

func (w *Writer) pruneArchives(base string) {
	if w.opts.MaxHistory <= 0 {
		return
	}
	matches, err := filepath.Glob(
		filepath.Join(w.opts.Directory, base+"_*"+filepath.Ext(w.opts.FileName)),
	)
	if err != nil || len(matches) <= w.opts.MaxHistory {
		return
	}
	sort.Strings(matches) // timestamp suffixes sort chronologically
	for _, old := range matches[:len(matches)-w.opts.MaxHistory] {
		_ = os.Remove(old)
	}
}

// Close flushes and closes the log file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	if err != nil {
		return fmt.Errorf("logging: closing log file: %w", err)
	}
	return nil
}
