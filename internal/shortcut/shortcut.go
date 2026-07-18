// Package shortcut creates, reads and updates Windows shortcuts: .lnk shell
// links (via IShellLinkW COM on Windows) and .url internet shortcuts (plain
// text, handled portably).
package shortcut

import (
	"fmt"
	"os"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// WindowStyle mirrors the shortcut show-command options PSADT accepts.
type WindowStyle int

// WindowStyle values (SW_* show commands).
const (
	WindowStyleNormal    WindowStyle = 1 // SW_SHOWNORMAL
	WindowStyleMaximized WindowStyle = 3 // SW_SHOWMAXIMIZED
	WindowStyleMinimized WindowStyle = 7 // SW_SHOWMINNOACTIVE
)

// ParseWindowStyle maps PSADT's WindowStyle strings.
func ParseWindowStyle(s string) (WindowStyle, error) {
	switch strings.ToLower(s) {
	case "", "normal":
		return WindowStyleNormal, nil
	case "maximized":
		return WindowStyleMaximized, nil
	case "minimized":
		return WindowStyleMinimized, nil
	default:
		return WindowStyleNormal, winerr.Wrap("shortcut: WindowStyle "+s, winerr.ErrInvalidOption)
	}
}

// Shortcut models the properties PSADT's *-ADTShortcut functions expose.
type Shortcut struct {
	Path             string // the .lnk/.url file itself
	TargetPath       string // .lnk target or .url URL
	Arguments        string // .lnk only
	Description      string // .lnk only
	WorkingDirectory string // .lnk only
	IconLocation     string
	IconIndex        int
	WindowStyle      WindowStyle // .lnk only
	Hotkey           string      // .lnk only, e.g. "Ctrl+Alt+F9"
}

// IsURL reports whether the shortcut path is an internet shortcut.
func (s *Shortcut) IsURL() bool {
	return strings.EqualFold(strings.TrimSpace(pathExt(s.Path)), ".url")
}

func pathExt(p string) string {
	if i := strings.LastIndexByte(p, '.'); i >= 0 {
		return p[i:]
	}
	return ""
}

// writeURLFile renders the InternetShortcut format PSADT writes.
func writeURLFile(s *Shortcut) error {
	var b strings.Builder
	b.WriteString("[InternetShortcut]\r\n")
	b.WriteString("URL=" + s.TargetPath + "\r\n")
	if s.IconLocation != "" {
		fmt.Fprintf(&b, "IconIndex=%d\r\n", s.IconIndex)
		b.WriteString("IconFile=" + s.IconLocation + "\r\n")
	}
	//#nosec G306 -- shortcuts are shared artifacts (e.g. common desktop) and must be world-readable
	if err := os.WriteFile(s.Path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("shortcut: writing url file: %w", err)
	}
	return nil
}

// readURLFile parses an InternetShortcut file.
func readURLFile(path string) (*Shortcut, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("shortcut: reading url file: %w", err)
	}
	s := &Shortcut{Path: path}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "url":
			s.TargetPath = strings.TrimSpace(value)
		case "iconfile":
			s.IconLocation = strings.TrimSpace(value)
		case "iconindex":
			_, _ = fmt.Sscanf(strings.TrimSpace(value), "%d", &s.IconIndex)
		}
	}
	return s, nil
}

// Create writes a new shortcut file (.url portably, .lnk via COM on Windows).
func Create(s *Shortcut) error {
	if s.Path == "" || s.TargetPath == "" {
		return winerr.Wrap("shortcut: Path and TargetPath are required", winerr.ErrInvalidOption)
	}
	if s.IsURL() {
		return writeURLFile(s)
	}
	return writeLinkFile(s)
}

// Read loads an existing shortcut's properties.
func Read(path string) (*Shortcut, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("shortcut: %s: %w", path, winerr.ErrNotFound)
	}
	if strings.EqualFold(pathExt(path), ".url") {
		return readURLFile(path)
	}
	return readLinkFile(path)
}

// Update applies the non-zero fields of changes to an existing shortcut.
func Update(path string, mutate func(current *Shortcut)) error {
	current, err := Read(path)
	if err != nil {
		return err
	}
	mutate(current)
	current.Path = path
	if current.IsURL() {
		return writeURLFile(current)
	}
	return writeLinkFile(current)
}
