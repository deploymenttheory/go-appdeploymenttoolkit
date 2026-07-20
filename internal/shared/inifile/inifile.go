// Package inifile implements Windows private-profile style INI file access
// in pure Go for the Get/Set/Remove-ADTIni* facade functions.
//
// Deviation from PSADT: PSADT's PSADT.Utilities.IniUtilities calls the Win32
// GetPrivateProfileString/WritePrivateProfileString family. This package is a
// pure-Go reimplementation so the behavior is portable and unit-testable on
// every platform. It is behavior-compatible for well-formed files:
//
//   - sections are "[Name]" lines, keys are "key=value" lines;
//   - section and key lookup is case-insensitive;
//   - duplicate keys resolve last-wins;
//   - values may contain '=' (only the first '=' splits key from value);
//   - ';' lines are comments;
//   - ReadValue strips one pair of surrounding single or double quotes
//     (GetPrivateProfileString parity); ReadSection returns raw values
//     (GetPrivateProfileSection parity);
//   - unknown lines and line ordering are preserved on write;
//   - files are written with CRLF line endings in the encoding they were
//     read in (UTF-8, UTF-8 with BOM, or UTF-16LE with BOM); new files are
//     written as UTF-8 without a BOM.
package inifile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
	"unicode/utf16"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// fileEncoding identifies the on-disk text encoding of an INI file.
type fileEncoding int

const (
	encodingUTF8 fileEncoding = iota
	encodingUTF8BOM
	encodingUTF16LE
)

// document is a parsed INI file: the raw lines (without line endings) plus
// the encoding the file was read in so writes round-trip byte-compatibly.
type document struct {
	encoding fileEncoding
	lines    []string
}

// load reads and parses an INI file. The caller decides how to treat a
// missing file (match with errors.Is(err, fs.ErrNotExist)).
func load(path string) (*document, error) {
	raw, err := os.ReadFile(path) //#nosec G304 -- the INI path is caller-supplied by design
	if err != nil {
		return nil, fmt.Errorf("inifile: reading %s: %w", path, err)
	}
	return parse(raw), nil
}

// loadOrEmpty reads an INI file, treating a missing file as an empty one.
func loadOrEmpty(path string) (*document, error) {
	doc, err := load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &document{}, nil
		}
		return nil, err
	}
	return doc, nil
}

// parse decodes raw file bytes into a document, detecting a UTF-16LE or
// UTF-8 byte-order mark.
func parse(raw []byte) *document {
	doc := &document{}
	var text string
	switch {
	case len(raw) >= 2 && raw[0] == 0xFF && raw[1] == 0xFE:
		doc.encoding = encodingUTF16LE
		text = decodeUTF16LE(raw[2:])
	case len(raw) >= 3 && raw[0] == 0xEF && raw[1] == 0xBB && raw[2] == 0xBF:
		doc.encoding = encodingUTF8BOM
		text = string(raw[3:])
	default:
		doc.encoding = encodingUTF8
		text = string(raw)
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return doc
	}
	doc.lines = strings.Split(text, "\n")
	return doc
}

// save writes the document back to disk with CRLF line endings in the
// encoding the file was originally read in.
func (d *document) save(path string) error {
	text := ""
	if len(d.lines) > 0 {
		text = strings.Join(d.lines, "\r\n") + "\r\n"
	}
	var raw []byte
	switch d.encoding {
	case encodingUTF16LE:
		raw = encodeUTF16LE(text)
	case encodingUTF8BOM:
		raw = append([]byte{0xEF, 0xBB, 0xBF}, text...)
	default:
		raw = []byte(text)
	}
	err := os.WriteFile(
		path,
		raw,
		0o644,
	) //#nosec G306 -- INI files are shared configuration, not secrets
	if err != nil {
		return fmt.Errorf("inifile: writing %s: %w", path, err)
	}
	return nil
}

// decodeUTF16LE decodes UTF-16 little-endian bytes (BOM already stripped).
func decodeUTF16LE(raw []byte) string {
	units := make([]uint16, 0, len(raw)/2)
	for i := 0; i+1 < len(raw); i += 2 {
		units = append(units, uint16(raw[i])|uint16(raw[i+1])<<8)
	}
	return string(utf16.Decode(units))
}

// encodeUTF16LE encodes text as UTF-16 little-endian with a leading BOM.
func encodeUTF16LE(text string) []byte {
	units := utf16.Encode([]rune(text))
	raw := make([]byte, 0, 2+len(units)*2)
	raw = append(raw, 0xFF, 0xFE)
	for _, u := range units {
		raw = append(
			raw,
			byte(u),
			byte(u>>8),
		) //#nosec G115 -- deliberate little-endian byte split of a UTF-16 code unit
	}
	return raw
}

// parseSectionHeader recognizes a "[Name]" line and returns the section name.
func parseSectionHeader(line string) (name string, ok bool) {
	t := strings.TrimSpace(line)
	if len(t) < 2 || t[0] != '[' {
		return "", false
	}
	end := strings.IndexByte(t, ']')
	if end <= 0 {
		return "", false
	}
	return strings.TrimSpace(t[1:end]), true
}

// parseKeyValue recognizes a "key=value" line; comments ([;] prefix),
// section headers, blank lines and lines with an empty key are rejected.
// Key and value are whitespace-trimmed; the value keeps any quotes.
func parseKeyValue(line string) (key, value string, ok bool) {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, ";") {
		return "", "", false
	}
	if _, isHeader := parseSectionHeader(t); isHeader {
		return "", "", false
	}
	k, v, found := strings.Cut(t, "=")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(k)
	if key == "" {
		return "", "", false
	}
	return key, strings.TrimSpace(v), true
}

// sectionRange locates section (case-insensitive) and returns the index of
// its header line and the exclusive end index of its body (the next section
// header or end of file).
func sectionRange(lines []string, section string) (header, end int, found bool) {
	for i, line := range lines {
		name, ok := parseSectionHeader(line)
		if !ok || !strings.EqualFold(name, section) {
			continue
		}
		end = len(lines)
		for j := i + 1; j < len(lines); j++ {
			if _, isHeader := parseSectionHeader(lines[j]); isHeader {
				end = j
				break
			}
		}
		return i, end, true
	}
	return 0, 0, false
}

// unquote strips one pair of surrounding single or double quotes, mirroring
// GetPrivateProfileString.
func unquote(v string) string {
	if len(v) >= 2 &&
		((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
		return v[1 : len(v)-1]
	}
	return v
}

// ReadValue returns the value of key within section. The error wraps
// errs.ErrNotFound when the file, section or key does not exist.
func ReadValue(path, section, key string) (string, error) {
	doc, err := load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", errs.Wrap("inifile: file "+path, errs.ErrNotFound)
		}
		return "", err
	}
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		return "", errs.Wrap("inifile: section "+section, errs.ErrNotFound)
	}
	value, matched := "", false
	for _, line := range doc.lines[header+1 : end] {
		if k, v, ok := parseKeyValue(line); ok && strings.EqualFold(k, key) {
			value, matched = v, true
		}
	}
	if !matched {
		return "", errs.Wrap("inifile: key "+key, errs.ErrNotFound)
	}
	return unquote(value), nil
}

// ReadSection returns all key=value pairs of section (last-wins for
// duplicate keys). The error wraps errs.ErrNotFound when the file or the
// section does not exist.
func ReadSection(path, section string) (map[string]string, error) {
	doc, err := load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errs.Wrap("inifile: file "+path, errs.ErrNotFound)
		}
		return nil, err
	}
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		return nil, errs.Wrap("inifile: section "+section, errs.ErrNotFound)
	}
	out := map[string]string{}
	for _, line := range doc.lines[header+1 : end] {
		if k, v, ok := parseKeyValue(line); ok {
			out[k] = v
		}
	}
	return out, nil
}

// ReadSectionNames returns the section names of the file in order of
// appearance. The error wraps errs.ErrNotFound when the file is missing.
func ReadSectionNames(path string) ([]string, error) {
	doc, err := load(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errs.Wrap("inifile: file "+path, errs.ErrNotFound)
		}
		return nil, err
	}
	names := make([]string, 0, len(doc.lines))
	for _, line := range doc.lines {
		if name, ok := parseSectionHeader(line); ok {
			names = append(names, name)
		}
	}
	return names, nil
}

// WriteValue sets key to value within section, creating the file and the
// section as needed. The first existing occurrence of the key is updated in
// place; any duplicate occurrences are removed.
func WriteValue(path, section, key, value string) error {
	doc, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	entry := key + "=" + value
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		doc.lines = append(doc.lines, "["+section+"]", entry)
		return doc.save(path)
	}
	replaced := false
	kept := make([]string, 0, len(doc.lines))
	for i, line := range doc.lines {
		if i > header && i < end {
			if k, _, ok := parseKeyValue(line); ok && strings.EqualFold(k, key) {
				if !replaced {
					kept = append(kept, entry)
					replaced = true
				}
				continue
			}
		}
		kept = append(kept, line)
	}
	if replaced {
		doc.lines = kept
		return doc.save(path)
	}
	// Insert at the end of the section body, before any blank separator
	// lines that precede the next section.
	insert := end
	for insert > header+1 && strings.TrimSpace(doc.lines[insert-1]) == "" {
		insert--
	}
	doc.lines = append(doc.lines[:insert], append([]string{entry}, doc.lines[insert:]...)...)
	return doc.save(path)
}

// WriteSection replaces the entire body of section with the given content
// (keys written in sorted order for determinism), creating the file and the
// section as needed. Mirrors WritePrivateProfileSection: comments within an
// existing section body are not preserved.
func WriteSection(path, section string, content map[string]string) error {
	doc, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(content))
	for k := range content {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	body := make([]string, 0, len(keys)+1)
	body = append(body, "["+section+"]")
	for _, k := range keys {
		body = append(body, k+"="+content[k])
	}
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		doc.lines = append(doc.lines, body...)
		return doc.save(path)
	}
	// Keep blank separator lines that trail the section body.
	bodyEnd := end
	for bodyEnd > header+1 && strings.TrimSpace(doc.lines[bodyEnd-1]) == "" {
		bodyEnd--
	}
	rebuilt := make([]string, 0, len(doc.lines[:header])+len(body)+len(doc.lines[bodyEnd:]))
	rebuilt = append(rebuilt, doc.lines[:header]...)
	rebuilt = append(rebuilt, body...)
	rebuilt = append(rebuilt, doc.lines[bodyEnd:]...)
	doc.lines = rebuilt
	return doc.save(path)
}

// DeleteValue removes every occurrence of key within section. Missing
// files, sections or keys are treated as already deleted (nil error).
func DeleteValue(path, section, key string) error {
	doc, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		return nil
	}
	changed := false
	kept := make([]string, 0, len(doc.lines))
	for i, line := range doc.lines {
		if i > header && i < end {
			if k, _, ok := parseKeyValue(line); ok && strings.EqualFold(k, key) {
				changed = true
				continue
			}
		}
		kept = append(kept, line)
	}
	if !changed {
		return nil
	}
	doc.lines = kept
	return doc.save(path)
}

// DeleteSection removes section and its entire body. Missing files or
// sections are treated as already deleted (nil error).
func DeleteSection(path, section string) error {
	doc, err := loadOrEmpty(path)
	if err != nil {
		return err
	}
	header, end, found := sectionRange(doc.lines, section)
	if !found {
		return nil
	}
	doc.lines = append(doc.lines[:header], doc.lines[end:]...)
	return doc.save(path)
}
