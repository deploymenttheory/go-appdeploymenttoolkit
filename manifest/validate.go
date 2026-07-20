package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateOptions configures the post-schema validation layers.
type ValidateOptions struct {
	// PackageDir enables file-existence checks for PathPackageFile params
	// against <PackageDir>/Files/. Empty disables the layer.
	PackageDir string
	// Target is the platform steps must support (default PlatformWindows).
	Target Platform
}

// Validate runs the semantic, package-dir and platform layers over a loaded
// manifest and returns the collected issues (schema issues are produced by
// Load). Nothing stops at the first problem.
func Validate(m *Manifest, opts ValidateOptions) []Issue {
	if opts.Target == "" {
		opts.Target = PlatformWindows
	}
	var issues []Issue

	// Session semantic checks.
	sessionAdd := makeAdd(&issues, "session", m.Session, Pos{Line: 1, Col: 1})
	checkSession(m.Session, sessionAdd)

	for _, phase := range m.Phases {
		for i, step := range phase.Steps {
			stepPath := fmt.Sprintf("phases.%s[%d]", phase.Name, i)
			spec, ok := Lookup(step.Uses)
			if !ok {
				continue // Load already reported unknown-step
			}
			// Layer b: per-step semantic checks.
			if spec.Check != nil {
				spec.Check(step.With, makeAdd(&issues, stepPath+".with", step.With, step.Pos))
			}
			// Layer c: package-dir file existence.
			if opts.PackageDir != "" {
				checkPackageFiles(&issues, stepPath+".with", spec, step.With, opts.PackageDir)
			}
			// Layer d: platform support.
			if !spec.SupportsPlatform(opts.Target) {
				addIssue(&issues, SeverityError, CodePlatformUnsupported, stepPath+".uses", step.UsesPos,
					"step %q is not available on %s (platforms: %s)",
					step.Uses, opts.Target, platformList(spec.Platforms))
			}
		}
	}
	SortIssues(issues)
	return issues
}

// makeAdd builds the AddIssue callback for a Check func: issues anchor at the
// named parameter's position when present, else at the fallback position.
func makeAdd(issues *[]Issue, basePath string, p Params, fallback Pos) AddIssue {
	return func(code, param, message string, warning bool) {
		sev := SeverityError
		if warning {
			sev = SeverityWarning
		}
		path := basePath
		pos := fallback
		if param != "" {
			path = basePath + "." + param
			if pp, ok := p.PosOf(param); ok {
				pos = pp
			}
		}
		addIssue(issues, sev, code, path, pos, "%s", message)
	}
}

// checkPackageFiles verifies PathPackageFile params reference existing files
// under <pkg>/Files/. Absolute paths, globs, %VAR% references and GUID-shaped
// values are skipped.
func checkPackageFiles(issues *[]Issue, basePath string, spec StepSpec, p Params, packageDir string) {
	filesDir := filepath.Join(packageDir, "Files")
	checkOne := func(paramPath, val string, pos Pos) {
		if val == "" || filepath.IsAbs(val) ||
			strings.ContainsAny(val, "*?[") || strings.Contains(val, "%") ||
			guidRe.MatchString(val) {
			return
		}
		if _, err := os.Stat(filepath.Join(filesDir, val)); err != nil {
			addIssue(issues, SeverityError, CodeMissingFile, paramPath, pos,
				"%q not found under %s", val, filesDir)
		}
	}
	for _, ps := range spec.Params {
		if ps.PathRole != PathPackageFile || !p.Has(ps.Name) {
			continue
		}
		pos, _ := p.PosOf(ps.Name)
		paramPath := basePath + "." + ps.Name
		if v, ok := p.String(ps.Name); ok {
			checkOne(paramPath, v, pos)
		}
		if vs, ok := p.StringList(ps.Name); ok {
			for i, v := range vs {
				checkOne(fmt.Sprintf("%s[%d]", paramPath, i), v, pos)
			}
		}
	}
}

// platformList renders a platform slice for messages.
func platformList(platforms []Platform) string {
	out := make([]string, len(platforms))
	for i, p := range platforms {
		out[i] = string(p)
	}
	return strings.Join(out, ", ")
}
