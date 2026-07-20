package manifest

import (
	"sort"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

// Platform tags the operating systems a step supports.
type Platform string

// Platform values. PlatformDarwin is reserved for the future macOS catalog.
const (
	PlatformWindows Platform = "windows"
	PlatformDarwin  Platform = "darwin"
)

// ParamType is the YAML-facing type of a step parameter.
type ParamType string

// ParamType values.
const (
	TypeString      ParamType = "string"
	TypeBool        ParamType = "bool"
	TypeInt         ParamType = "int"
	TypeFloat       ParamType = "float"
	TypeDuration    ParamType = "duration"  // Go duration string, e.g. "90s"
	TypeTimestamp   ParamType = "timestamp" // RFC 3339 or YYYY-MM-DD
	TypeStringList  ParamType = "stringList"
	TypeIntList     ParamType = "intList"
	TypeProcessList ParamType = "processList" // [{name, description?}]
	TypeAny         ParamType = "any"         // registry.set value only
)

// PathRole marks path-valued params for --package-dir existence checks.
type PathRole int

// PathRole values.
const (
	// PathNone is not a path (or not checkable).
	PathNone PathRole = iota
	// PathPackageFile resolves relative values against <pkg>/Files/ and is
	// existence-checked by validation layer (c); Compile normalizes relative
	// values to absolute Files/ paths so validation and runtime agree.
	PathPackageFile
	// PathMachine is a target-machine path; never checked.
	PathMachine
)

// ParamSpec describes one `with:` parameter of a step.
type ParamSpec struct {
	Name     string    `json:"name"`
	Type     ParamType `json:"type"`
	Required bool      `json:"required"`
	// Default is documentation for `adt steps`/the studio; it is not applied
	// by the decoder (absent params stay absent).
	Default any `json:"default,omitempty"`
	// Enum lists canonical allowed values (matched case-insensitively).
	Enum        []string `json:"enum,omitempty"`
	Description string   `json:"description"`
	PathRole    PathRole `json:"-"`
}

// Value is one decoded parameter value with its source position.
type Value struct {
	V   any
	Pos Pos
}

// Params is a decoded, schema-valid parameter set (a `with:` block or the
// `session:` block) with source positions retained.
type Params struct {
	values map[string]Value
}

// Has reports whether the parameter was present in the manifest.
func (p Params) Has(name string) bool { _, ok := p.values[name]; return ok }

// PosOf returns the source position of a present parameter.
func (p Params) PosOf(name string) (Pos, bool) {
	v, ok := p.values[name]
	return v.Pos, ok
}

// String returns a string parameter.
func (p Params) String(name string) (string, bool) {
	v, ok := p.values[name].V.(string)
	return v, ok && p.Has(name)
}

// StringOr returns the parameter or a fallback.
func (p Params) StringOr(name, fallback string) string {
	if v, ok := p.String(name); ok {
		return v
	}
	return fallback
}

// Bool returns a bool parameter.
func (p Params) Bool(name string) (bool, bool) {
	v, ok := p.values[name].V.(bool)
	return v, ok
}

// BoolOr returns the parameter or a fallback.
func (p Params) BoolOr(name string, fallback bool) bool {
	if v, ok := p.Bool(name); ok {
		return v
	}
	return fallback
}

// Int returns an int parameter.
func (p Params) Int(name string) (int, bool) {
	v, ok := p.values[name].V.(int)
	return v, ok
}

// IntOr returns the parameter or a fallback.
func (p Params) IntOr(name string, fallback int) int {
	if v, ok := p.Int(name); ok {
		return v
	}
	return fallback
}

// Float returns a float parameter.
func (p Params) Float(name string) (float64, bool) {
	v, ok := p.values[name].V.(float64)
	return v, ok
}

// Duration returns a duration parameter.
func (p Params) Duration(name string) (time.Duration, bool) {
	v, ok := p.values[name].V.(time.Duration)
	return v, ok
}

// Time returns a timestamp parameter.
func (p Params) Time(name string) (time.Time, bool) {
	v, ok := p.values[name].V.(time.Time)
	return v, ok
}

// StringList returns a string-list parameter.
func (p Params) StringList(name string) ([]string, bool) {
	v, ok := p.values[name].V.([]string)
	return v, ok
}

// IntList returns an int-list parameter.
func (p Params) IntList(name string) ([]int, bool) {
	v, ok := p.values[name].V.([]int)
	return v, ok
}

// ProcessList returns a process-list parameter.
func (p Params) ProcessList(name string) ([]adt.ProcessObject, bool) {
	v, ok := p.values[name].V.([]adt.ProcessObject)
	return v, ok
}

// Any returns a raw parameter value (TypeAny params).
func (p Params) Any(name string) (any, bool) {
	v, ok := p.values[name]
	return v.V, ok
}

// Names returns the present parameter names (sorted, for deterministic use).
func (p Params) Names() []string {
	out := make([]string, 0, len(p.values))
	for k := range p.values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// set stores a decoded value (used by the decoder).
func (p *Params) set(name string, v Value) {
	if p.values == nil {
		p.values = map[string]Value{}
	}
	p.values[name] = v
}

// AddIssue is the callback step Check funcs use to report semantic findings
// against a named parameter (empty name anchors at the step itself).
type AddIssue func(code, param, message string, warning bool)

// StepSpec declares one step of the catalog: its name, parameter schema,
// platform support, semantic checks and the binding onto the adt engine.
type StepSpec struct {
	Name      string      `json:"name"`    // e.g. "msi.install"
	Summary   string      `json:"summary"` // one line for `adt steps`
	Platforms []Platform  `json:"platforms"`
	Params    []ParamSpec `json:"params"`
	// Check adds cross-field semantic issues (validation layer b). Nil = none.
	Check func(p Params, add AddIssue) `json:"-"`
	// Bind materializes the step into a phase fragment; Compile chains these
	// (honoring name/continueOnError) into the Deployment's PhaseFuncs. Bind
	// must not execute anything.
	Bind func(p Params) (adt.PhaseFunc, error) `json:"-"`
}

// SupportsPlatform reports whether the step is available on the target.
func (s StepSpec) SupportsPlatform(target Platform) bool {
	for _, p := range s.Platforms {
		if p == target {
			return true
		}
	}
	return false
}

// Param returns the named ParamSpec.
func (s StepSpec) Param(name string) (ParamSpec, bool) {
	for _, p := range s.Params {
		if p.Name == name {
			return p, true
		}
	}
	return ParamSpec{}, false
}

// catalog is the registered step set, populated by register() calls in the
// steps_*.go init functions.
var catalog = map[string]StepSpec{}

// register adds a step to the catalog; duplicate names panic at init time.
func register(s StepSpec) {
	if _, exists := catalog[s.Name]; exists {
		panic("manifest: duplicate step registration: " + s.Name)
	}
	catalog[s.Name] = s
}

// Steps returns the full catalog sorted by name.
func Steps() []StepSpec {
	out := make([]StepSpec, 0, len(catalog))
	for _, s := range catalog {
		out = append(out, s)
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out
}

// Lookup returns the named step.
func Lookup(name string) (StepSpec, bool) {
	s, ok := catalog[name]
	return s, ok
}
