package manifest

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/shortcut"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/svcmgmt"
)

// shortcutParams is the shortcut.create parameter table.
var shortcutParams = []ParamSpec{
	{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine,
		Description: "the .lnk or .url file to create"},
	{Name: "targetPath", Type: TypeString, Required: true, Description: ".lnk target or .url URL"},
	{Name: "arguments", Type: TypeString, Description: "target arguments (.lnk only)"},
	{Name: "description", Type: TypeString, Description: "shortcut description (.lnk only)"},
	{Name: "workingDirectory", Type: TypeString, Description: "target working directory (.lnk only)"},
	{Name: "iconLocation", Type: TypeString, Description: "icon file"},
	{Name: "iconIndex", Type: TypeInt, Description: "icon index within the icon file"},
	{Name: "windowStyle", Type: TypeString, Enum: []string{"normal", "maximized", "minimized"},
		Description: "launch window style (.lnk only)"},
	{Name: "hotkey", Type: TypeString, Description: `hotkey such as "Ctrl+Alt+F9" (.lnk only)`},
}

// shortcutFromParams maps shortcut params onto the Shortcut struct.
func shortcutFromParams(p Params) winadt.Shortcut {
	sc := winadt.Shortcut{
		Path:             p.StringOr("path", ""),
		TargetPath:       p.StringOr("targetPath", ""),
		Arguments:        p.StringOr("arguments", ""),
		Description:      p.StringOr("description", ""),
		WorkingDirectory: p.StringOr("workingDirectory", ""),
		IconLocation:     p.StringOr("iconLocation", ""),
		IconIndex:        p.IntOr("iconIndex", 0),
		Hotkey:           p.StringOr("hotkey", ""),
	}
	if style, ok := p.String("windowStyle"); ok {
		if ws, err := shortcut.ParseWindowStyle(style); err == nil {
			sc.WindowStyle = ws
		}
	}
	return sc
}

// checkShortcutURL warns about .lnk-only params on .url shortcuts.
func checkShortcutURL(p Params, add AddIssue) {
	path, _ := p.String("path")
	sc := winadt.Shortcut{Path: path}
	if !sc.IsURL() {
		return
	}
	for _, name := range []string{"arguments", "description", "workingDirectory", "windowStyle", "hotkey"} {
		if p.Has(name) {
			add(CodeSemantic, name, name+" only applies to .lnk shortcuts, not .url", true)
		}
	}
}

func init() {
	register(StepSpec{
		Name: "shortcut.create", Summary: "Create a .lnk or .url shortcut",
		Platforms: []Platform{PlatformWindows},
		Params:    shortcutParams,
		Check:     checkShortcutURL,
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			sc := shortcutFromParams(p)
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.NewADTShortcut(ctx, sc)
			}, nil
		},
	})
	register(StepSpec{
		Name: "shortcut.removeDesktop", Summary: "Remove a shortcut from the common desktop",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "name", Type: TypeString, Required: true,
				Description: `shortcut name (".lnk" appended when no extension)`},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			name := p.StringOr("name", "")
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTDesktopShortcut(ctx, name)
			}, nil
		},
	})
	register(StepSpec{
		Name: "service.start", Summary: "Start a service and its dependencies",
		Platforms: []Platform{PlatformWindows},
		Params:    serviceParams,
		Bind:      bindService(winadt.StartADTServiceAndDependencies),
	})
	register(StepSpec{
		Name: "service.stop", Summary: "Stop a service and its dependent services",
		Platforms: []Platform{PlatformWindows},
		Params:    serviceParams,
		Bind:      bindService(winadt.StopADTServiceAndDependencies),
	})
	register(StepSpec{
		Name: "service.setStartMode", Summary: "Set a service's startup mode",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "name", Type: TypeString, Required: true, Description: "service name"},
			{Name: "startMode", Type: TypeString, Required: true,
				Enum:        []string{"Automatic", "Automatic (Delayed Start)", "Manual", "Disabled"},
				Description: "startup mode"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			name := p.StringOr("name", "")
			mode, _ := svcmgmt.ParseStartMode(p.StringOr("startMode", ""))
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.SetADTServiceStartMode(ctx, name, mode)
			}, nil
		},
	})
	register(StepSpec{
		Name: "env.set", Summary: "Set an environment variable",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "variable", Type: TypeString, Required: true, Description: "variable name"},
			{Name: "value", Type: TypeString, Required: true, Description: "variable value"},
			{Name: "target", Type: TypeString, Enum: []string{"process", "user", "machine"},
				Description: "scope (default process)"},
			{Name: "expandable", Type: TypeBool, Description: "store as REG_EXPAND_SZ (registry targets)"},
		},
		Check: func(p Params, add AddIssue) {
			if p.BoolOr("expandable", false) && p.StringOr("target", "process") == "process" {
				add(CodeSemantic, "expandable", "expandable only applies to user/machine targets", true)
			}
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.SetADTEnvironmentVariableOptions{
				Variable:   p.StringOr("variable", ""),
				Value:      p.StringOr("value", ""),
				Target:     p.StringOr("target", ""),
				Expandable: p.BoolOr("expandable", false),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.SetADTEnvironmentVariable(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "env.remove", Summary: "Remove an environment variable",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "variable", Type: TypeString, Required: true, Description: "variable name"},
			{Name: "target", Type: TypeString, Enum: []string{"process", "user", "machine"},
				Description: "scope (default process)"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.RemoveADTEnvironmentVariableOptions{
				Variable: p.StringOr("variable", ""),
				Target:   p.StringOr("target", ""),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTEnvironmentVariable(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "fonts.add", Summary: "Install a font file",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "filePath", Type: TypeString, Required: true, PathRole: PathPackageFile,
				Description: "font file (relative to Files/)"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.AddADTFontOptions{FilePath: p.StringOr("filePath", "")}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.AddADTFont(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "fonts.remove", Summary: "Remove an installed font",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "name", Type: TypeString, Required: true, Description: "installed font name"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.RemoveADTFontOptions{Name: p.StringOr("name", "")}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTFont(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "dll.register", Summary: "Register a DLL (regsvr32)",
		Platforms: []Platform{PlatformWindows},
		Params:    dllParams,
		Bind:      bindDll(winadt.RegisterADTDll),
	})
	register(StepSpec{
		Name: "dll.unregister", Summary: "Unregister a DLL (regsvr32 /u)",
		Platforms: []Platform{PlatformWindows},
		Params:    dllParams,
		Bind:      bindDll(winadt.UnregisterADTDll),
	})
}

// serviceParams is shared by service.start and service.stop.
var serviceParams = []ParamSpec{
	{Name: "name", Type: TypeString, Required: true, Description: "service name"},
	{Name: "pendingStatusWait", Type: TypeDuration, Description: "how long to wait out a pending state"},
}

// bindService adapts the shared service signature.
func bindService(
	fn func(ctx context.Context, name string, opts ...winadt.StartADTServiceAndDependenciesOptions) error,
) func(p Params) (deploy.PhaseFunc, error) {
	return func(p Params) (deploy.PhaseFunc, error) {
		name := p.StringOr("name", "")
		var opts []winadt.StartADTServiceAndDependenciesOptions
		if wait, ok := p.Duration("pendingStatusWait"); ok {
			opts = append(opts, winadt.StartADTServiceAndDependenciesOptions{PendingStatusWait: wait})
		}
		return func(ctx context.Context, s *deploy.Session) error {
			return fn(ctx, name, opts...)
		}, nil
	}
}

// dllParams is shared by dll.register and dll.unregister.
var dllParams = []ParamSpec{
	{Name: "filePath", Type: TypeString, Required: true, PathRole: PathPackageFile,
		Description: "DLL to (un)register (relative to Files/)"},
}

// bindDll adapts the shared DLL signature.
func bindDll(fn func(ctx context.Context, filePath string) error) func(p Params) (deploy.PhaseFunc, error) {
	return func(p Params) (deploy.PhaseFunc, error) {
		path := p.StringOr("filePath", "")
		return func(ctx context.Context, s *deploy.Session) error {
			return fn(ctx, path)
		}, nil
	}
}
