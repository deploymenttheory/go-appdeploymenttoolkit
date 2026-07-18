// Package strtab provides PSAppDeployToolkit's localized string tables.
//
// The embedded tables are generated from PSADT's ImportsLast.ps1 by
// tools/psd1convert: strings.default.json is the culture-neutral (English)
// table and strings.<culture>.json are per-culture overlays for the 26
// PSADT-supported cultures. Tables are nested maps addressed by
// backslash-free dotted paths (e.g. "CloseAppsPrompt.Fluent.ButtonRightText");
// leaves may be plain strings or per-deployment-type maps keyed by
// Install/Uninstall/Repair.
package strtab

import (
	"embed"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed embedded/strings.*.json
var embedded embed.FS

// Table is a resolved string table for one culture.
type Table struct {
	root map[string]any
}

// Cultures returns the culture codes with embedded overlay tables.
func Cultures() []string {
	entries, err := embedded.ReadDir("embedded")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		name := strings.TrimSuffix(strings.TrimPrefix(e.Name(), "strings."), ".json")
		if name != "default" {
			out = append(out, name)
		}
	}
	return out
}

// Load builds the table for the requested culture: culture-neutral defaults,
// deep-merged with the closest embedded culture overlay (exact match first,
// then the parent culture, e.g. "pt-BR" falls back to "pt"), then an optional
// package overlay YAML file. Empty culture yields the defaults.
func Load(culture, overlayPath string) (*Table, error) {
	root, err := readEmbedded("default")
	if err != nil {
		return nil, err
	}
	for _, candidate := range cultureChain(culture) {
		overlay, err := readEmbedded(candidate)
		if err != nil {
			continue // no table for this culture; try next in chain
		}
		merge(root, overlay)
		break
	}
	if overlayPath != "" {
		data, err := os.ReadFile(overlayPath)
		if err != nil {
			return nil, fmt.Errorf("strtab: reading overlay: %w", err)
		}
		custom := map[string]any{}
		if err := yaml.Unmarshal(data, &custom); err != nil {
			return nil, fmt.Errorf("strtab: parsing overlay %s: %w", overlayPath, err)
		}
		merge(root, custom)
	}
	return &Table{root: root}, nil
}

// cultureChain returns lookup candidates from most to least specific:
// "zh-HK" -> ["zh-HK", "zh"], "en" -> ["en"], "" -> nil.
func cultureChain(culture string) []string {
	if culture == "" {
		return nil
	}
	chain := []string{culture}
	if parent, _, ok := strings.Cut(culture, "-"); ok {
		chain = append(chain, parent)
	}
	return chain
}

func readEmbedded(name string) (map[string]any, error) {
	data, err := embedded.ReadFile("embedded/strings." + name + ".json")
	if err != nil {
		return nil, fmt.Errorf("strtab: no embedded table %q: %w", name, err)
	}
	m := map[string]any{}
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("strtab: parsing embedded table %q: %w", name, err)
	}
	return m, nil
}

// merge deep-merges src into dst: maps merge recursively, everything else
// (including slices) replaces. Null overlay values are ignored so sparse
// culture tables never blank out default text.
func merge(dst, src map[string]any) {
	for k, v := range src {
		if v == nil {
			continue
		}
		if sm, ok := v.(map[string]any); ok {
			if dm, ok := dst[k].(map[string]any); ok {
				merge(dm, sm)
				continue
			}
		}
		dst[k] = v
	}
}

// Get resolves a dotted path to a string leaf. When the addressed node is a
// per-deployment-type map, deploymentType ("Install", "Uninstall", "Repair")
// selects the leaf. Returns "" and false when the path does not resolve.
func (t *Table) Get(path, deploymentType string) (string, bool) {
	var node any = t.root
	for _, part := range strings.Split(path, ".") {
		m, ok := node.(map[string]any)
		if !ok {
			return "", false
		}
		if node, ok = m[part]; !ok {
			return "", false
		}
	}
	if m, ok := node.(map[string]any); ok {
		var okLeaf bool
		if node, okLeaf = m[deploymentType]; !okLeaf {
			return "", false
		}
	}
	s, ok := node.(string)
	return s, ok
}

// MustGet is Get returning the path itself when unresolved, so missing
// strings surface visibly in UI rather than as empty text.
func (t *Table) MustGet(path, deploymentType string) string {
	if s, ok := t.Get(path, deploymentType); ok {
		return s
	}
	return path
}

// Interpolate replaces `{Section\Key}` config references in s using lookup,
// mirroring PSADT's string interpolation (e.g. `{Toolkit\CompanyName}`).
func Interpolate(s string, lookup func(ref string) (string, bool)) string {
	var b strings.Builder
	for {
		start := strings.IndexByte(s, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(s[start:], '}')
		if end < 0 {
			break
		}
		ref := s[start+1 : start+end]
		b.WriteString(s[:start])
		if v, ok := lookup(ref); ok {
			b.WriteString(v)
		} else {
			b.WriteString(s[start : start+end+1])
		}
		s = s[start+end+1:]
	}
	b.WriteString(s)
	return b.String()
}
