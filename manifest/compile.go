package manifest

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/session"
)

// CompileOptions configures manifest compilation.
type CompileOptions struct {
	// PackageDir is the package root (ScriptDirectory); Files/, Config/ and
	// Strings/ are resolved beneath it. Defaults to the manifest's directory.
	PackageDir string
}

// Compile materializes a loaded, validated manifest into an adt.Deployment:
// the session block becomes SessionOptions and each phase's steps are bound
// and chained into a PhaseFunc that logs step progress and honors
// continueOnError. Compile refuses manifests with error-severity issues.
func Compile(m *Manifest, opts CompileOptions) (*adt.Deployment, error) {
	packageDir := opts.PackageDir
	if packageDir == "" {
		packageDir = filepath.Dir(m.Path)
	}
	if issues := Validate(m, ValidateOptions{PackageDir: packageDir}); HasErrors(issues) {
		return nil, fmt.Errorf("manifest: %s has validation errors; run `adt validate` for details", m.Path)
	}

	dep := &adt.Deployment{Session: sessionOptionsFromParams(m.Session, packageDir)}
	for _, phase := range m.Phases {
		if len(phase.Steps) == 0 {
			continue
		}
		fn, err := compilePhase(phase, packageDir)
		if err != nil {
			return nil, err
		}
		switch phase.Name {
		case "preInstall":
			dep.PreInstall = fn
		case "install":
			dep.Install = fn
		case "postInstall":
			dep.PostInstall = fn
		case "preUninstall":
			dep.PreUninstall = fn
		case "uninstall":
			dep.Uninstall = fn
		case "postUninstall":
			dep.PostUninstall = fn
		case "preRepair":
			dep.PreRepair = fn
		case "repair":
			dep.Repair = fn
		case "postRepair":
			dep.PostRepair = fn
		}
	}
	return dep, nil
}

// LoadAndCompile is the CLI's one-call path: Load, Validate (refusing
// errors), Compile.
func LoadAndCompile(path string, opts CompileOptions) (*adt.Deployment, []Issue, error) {
	m, issues, err := Load(path)
	if err != nil {
		return nil, nil, err
	}
	if opts.PackageDir == "" {
		opts.PackageDir = filepath.Dir(path)
	}
	issues = append(issues, Validate(m, ValidateOptions{PackageDir: opts.PackageDir})...)
	SortIssues(issues)
	if HasErrors(issues) {
		return nil, issues, fmt.Errorf("manifest: %s has validation errors", path)
	}
	dep, err := Compile(m, opts)
	return dep, issues, err
}

// compilePhase binds the phase's steps and chains them into one PhaseFunc.
func compilePhase(phase Phase, packageDir string) (adt.PhaseFunc, error) {
	type boundStep struct {
		label           string
		uses            string
		continueOnError bool
		run             adt.PhaseFunc
	}
	bound := make([]boundStep, 0, len(phase.Steps))
	for i, step := range phase.Steps {
		spec, ok := Lookup(step.Uses)
		if !ok {
			return nil, fmt.Errorf("manifest: phases.%s[%d]: unknown step %q", phase.Name, i, step.Uses)
		}
		params := normalizePackagePaths(step.With, spec, packageDir)
		fn, err := spec.Bind(params)
		if err != nil {
			return nil, fmt.Errorf("manifest: phases.%s[%d] (%s): %w", phase.Name, i, step.Uses, err)
		}
		label := step.DisplayName
		if label == "" {
			label = step.Uses
		}
		bound = append(bound, boundStep{
			label:           label,
			uses:            step.Uses,
			continueOnError: step.ContinueOnError,
			run:             fn,
		})
	}
	return func(ctx context.Context, s *adt.DeploymentSession) error {
		for i, b := range bound {
			s.WriteLog(
				fmt.Sprintf("Running step [%d/%d] [%s] (%s).", i+1, len(bound), b.label, b.uses),
				adt.LogSeverityInfo, "manifest", "",
			)
			if err := b.run(ctx, s); err != nil {
				if b.continueOnError {
					s.WriteLog(
						fmt.Sprintf("Step [%s] failed but continueOnError is set: %v", b.label, err),
						adt.LogSeverityWarning, "manifest", "",
					)
					continue
				}
				return fmt.Errorf("step %q: %w", b.label, err)
			}
		}
		return nil
	}, nil
}

// normalizePackagePaths rewrites relative PathPackageFile params to absolute
// paths under <packageDir>/Files/ so runtime resolution matches validation's
// existence checks regardless of which SDK function consumes the value.
// GUIDs, absolute paths, globs and %VAR% references pass through.
func normalizePackagePaths(p Params, spec StepSpec, packageDir string) Params {
	if packageDir == "" {
		return p
	}
	filesDir := filepath.Join(packageDir, "Files")
	normalize := func(v string) string {
		if v == "" || filepath.IsAbs(v) ||
			strings.ContainsAny(v, "*?[") || strings.Contains(v, "%") ||
			guidRe.MatchString(v) {
			return v
		}
		candidate := filepath.Join(filesDir, v)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		return v
	}
	for _, ps := range spec.Params {
		if ps.PathRole != PathPackageFile || !p.Has(ps.Name) {
			continue
		}
		pos, _ := p.PosOf(ps.Name)
		if v, ok := p.String(ps.Name); ok {
			p.set(ps.Name, Value{V: normalize(v), Pos: pos})
		}
		if vs, ok := p.StringList(ps.Name); ok {
			out := make([]string, len(vs))
			for i, v := range vs {
				out[i] = normalize(v)
			}
			p.set(ps.Name, Value{V: out, Pos: pos})
		}
	}
	return p
}

// sessionOptionsFromParams maps the session block onto adt.SessionOptions.
func sessionOptionsFromParams(p Params, packageDir string) adt.SessionOptions {
	opts := adt.SessionOptions{
		AppVendor:              p.StringOr("appVendor", ""),
		AppName:                p.StringOr("appName", ""),
		AppVersion:             p.StringOr("appVersion", ""),
		AppArch:                p.StringOr("appArch", ""),
		AppLang:                p.StringOr("appLang", ""),
		AppRevision:            p.StringOr("appRevision", ""),
		InstallName:            p.StringOr("installName", ""),
		InstallTitle:           p.StringOr("installTitle", ""),
		LogName:                p.StringOr("logName", ""),
		DisableLogging:         p.BoolOr("disableLogging", false),
		SuppressRebootPassThru: p.BoolOr("suppressRebootPassThru", false),
		TerminalServerMode:     p.BoolOr("terminalServerMode", false),
		RequireAdmin:           p.BoolOr("requireAdmin", false),
		LanguageOverride:       p.StringOr("languageOverride", ""),

		NoOobeDetection:               p.BoolOr("noOobeDetection", false),
		NoProcessDetection:            p.BoolOr("noProcessDetection", false),
		NoSessionDetection:            p.BoolOr("noSessionDetection", false),
		ProcessInteractivityDetection: p.BoolOr("processInteractivityDetection", false),

		ScriptDirectory:    packageDir,
		ConfigOverlayPath:  optionalOverlay(packageDir, "Config", "config.yaml"),
		StringsOverlayPath: optionalOverlay(packageDir, "Strings", "strings.yaml"),
	}
	if mode, ok := p.String("deployMode"); ok {
		if m, valid := session.ParseDeployMode(mode); valid {
			opts.DeployMode = m
		}
	}
	if v, ok := p.ProcessList("closeProcesses"); ok {
		opts.AppProcessesToClose = v
	}
	if v, ok := p.IntList("successExitCodes"); ok {
		opts.AppSuccessExitCodes = v
	}
	if v, ok := p.IntList("rebootExitCodes"); ok {
		opts.AppRebootExitCodes = v
	}
	return opts
}

// optionalOverlay returns the conventional overlay path when it exists.
func optionalOverlay(packageDir string, parts ...string) string {
	p := filepath.Join(append([]string{packageDir}, parts...)...)
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}
