package regkey

import (
	"sort"
	"strings"
	"sync"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Fake is an in-memory Backend for tests and non-Windows tooling.
type Fake struct {
	mu   sync.Mutex
	keys map[string]map[string]Value // "HIVE\path" -> name -> value
}

// NewFake returns an empty in-memory registry.
func NewFake() *Fake {
	return &Fake{keys: map[string]map[string]Value{}}
}

func fkey(hive, path string) string {
	return strings.ToUpper(hive) + `\` + strings.ToLower(path)
}

// GetValue implements Backend.
func (f *Fake) GetValue(hive, path, name string) (Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	vals, ok := f.keys[fkey(hive, path)]
	if !ok {
		return Value{}, winerr.Wrap("fake: key "+path, winerr.ErrNotFound)
	}
	v, ok := vals[name]
	if !ok {
		return Value{}, winerr.Wrap("fake: value "+name, winerr.ErrNotFound)
	}
	return v, nil
}

// SetValue implements Backend.
func (f *Fake) SetValue(hive, path, name string, value Value) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := fkey(hive, path)
	if f.keys[k] == nil {
		f.keys[k] = map[string]Value{}
	}
	f.keys[k][name] = value
	return nil
}

// DeleteValue implements Backend.
func (f *Fake) DeleteValue(hive, path, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	vals, ok := f.keys[fkey(hive, path)]
	if !ok {
		return winerr.Wrap("fake: key "+path, winerr.ErrNotFound)
	}
	if _, ok := vals[name]; !ok {
		return winerr.Wrap("fake: value "+name, winerr.ErrNotFound)
	}
	delete(vals, name)
	return nil
}

// DeleteKey implements Backend.
func (f *Fake) DeleteKey(hive, path string, recurse bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := fkey(hive, path)
	if _, ok := f.keys[k]; !ok {
		return winerr.Wrap("fake: key "+path, winerr.ErrNotFound)
	}
	delete(f.keys, k)
	if recurse {
		prefix := k + `\`
		for key := range f.keys {
			if strings.HasPrefix(key, prefix) {
				delete(f.keys, key)
			}
		}
	}
	return nil
}

// CreateKey implements Backend.
func (f *Fake) CreateKey(hive, path string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := fkey(hive, path)
	if f.keys[k] == nil {
		f.keys[k] = map[string]Value{}
	}
	return nil
}

// KeyExists implements Backend.
func (f *Fake) KeyExists(hive, path string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.keys[fkey(hive, path)]
	return ok, nil
}

// EnumValues implements Backend.
func (f *Fake) EnumValues(hive, path string) (map[string]Value, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	vals, ok := f.keys[fkey(hive, path)]
	if !ok {
		return nil, winerr.Wrap("fake: key "+path, winerr.ErrNotFound)
	}
	out := make(map[string]Value, len(vals))
	for k, v := range vals {
		out[k] = v
	}
	return out, nil
}

// EnumSubkeys implements Backend.
func (f *Fake) EnumSubkeys(hive, path string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	prefix := fkey(hive, path) + `\`
	seen := map[string]bool{}
	for key := range f.keys {
		if strings.HasPrefix(key, prefix) {
			rest := strings.TrimPrefix(key, prefix)
			first, _, _ := strings.Cut(rest, `\`)
			seen[first] = true
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}
