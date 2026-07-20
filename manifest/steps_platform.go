package manifest

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

func init() {
	register(StepSpec{
		Name: "activeSetup.set", Summary: "Create an Active Setup entry that runs per user at logon",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "stubExePath", Type: TypeString, Required: true, PathRole: PathMachine,
				Description: "the stub executed per user (.exe/.vbs/.cmd/.bat/.ps1/.js)"},
			{Name: "arguments", Type: TypeStringList, Description: "stub arguments"},
			{Name: "key", Type: TypeString, Description: "Installed Components key name (default derived)"},
			{Name: "description", Type: TypeString, Description: "entry description"},
			{Name: "version", Type: TypeString, Description: "Active Setup version stamp"},
			{Name: "locale", Type: TypeString, Description: "entry locale"},
			{Name: "executionPolicy", Type: TypeString, Description: "PowerShell execution policy for .ps1 stubs"},
			{Name: "wow6432Node", Type: TypeBool, Description: "write the 32-bit registry view"},
			{Name: "disableActiveSetup", Type: TypeBool, Description: "create the entry disabled"},
			{Name: "noExecuteForCurrentUser", Type: TypeBool, Description: "skip immediate execution for the current user"},
			{Name: "purge", Type: TypeBool, Description: "remove the entry (and per-user state) instead of creating it"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.SetADTActiveSetupOptions{
				StubExePath:             p.StringOr("stubExePath", ""),
				Key:                     p.StringOr("key", ""),
				Description:             p.StringOr("description", ""),
				Version:                 p.StringOr("version", ""),
				Locale:                  p.StringOr("locale", ""),
				ExecutionPolicy:         p.StringOr("executionPolicy", ""),
				Wow6432Node:             p.BoolOr("wow6432Node", false),
				DisableActiveSetup:      p.BoolOr("disableActiveSetup", false),
				NoExecuteForCurrentUser: p.BoolOr("noExecuteForCurrentUser", false),
				PurgeActiveSetupKey:     p.BoolOr("purge", false),
			}
			if v, ok := p.StringList("arguments"); ok {
				opts.ArgumentList = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.SetADTActiveSetup(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "edge.extensionAdd", Summary: "Add an Edge extension via the ExtensionSettings policy",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "extensionId", Type: TypeString, Required: true, Description: "extension ID"},
			{Name: "updateUrl", Type: TypeString, Required: true, Description: "extension update URL"},
			{Name: "installationMode", Type: TypeString, Required: true,
				Enum:        []string{"blocked", "allowed", "removed", "force_installed", "normal_installed"},
				Description: "policy installation mode"},
			{Name: "minimumVersionRequired", Type: TypeString, Description: "minimum extension version"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.AddADTEdgeExtensionOptions{
				ExtensionID:            p.StringOr("extensionId", ""),
				UpdateURL:              p.StringOr("updateUrl", ""),
				InstallationMode:       p.StringOr("installationMode", ""),
				MinimumVersionRequired: p.StringOr("minimumVersionRequired", ""),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.AddADTEdgeExtension(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "edge.extensionRemove", Summary: "Remove an Edge extension policy entry",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "extensionId", Type: TypeString, Required: true, Description: "extension ID"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.RemoveADTEdgeExtensionOptions{ExtensionID: p.StringOr("extensionId", "")}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.RemoveADTEdgeExtension(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "sccm.invokeTask", Summary: "Trigger a ConfigMgr client schedule task",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "scheduleId", Type: TypeString, Required: true,
				Description: "schedule name (e.g. HardwareInventory) or {GUID} schedule ID"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			id := p.StringOr("scheduleId", "")
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.InvokeADTSCCMTask(ctx, id)
			}, nil
		},
	})
	register(StepSpec{
		Name: "sccm.installUpdates", Summary: "Install pending ConfigMgr software updates",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.InstallADTSCCMSoftwareUpdates(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "gpo.update", Summary: "Run a group policy update (gpupdate)",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.UpdateADTGroupPolicy(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "terminalServer.enable", Summary: "Switch RDS/Citrix hosts to install mode",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.EnableADTTerminalServerInstallMode(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "terminalServer.disable", Summary: "Switch RDS/Citrix hosts back to execute mode",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.DisableADTTerminalServerInstallMode(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "wim.mount", Summary: "Mount a WIM image to a directory",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "imagePath", Type: TypeString, Required: true, PathRole: PathPackageFile,
				Description: "the .wim file (relative to Files/)"},
			{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine,
				Description: "mount directory"},
			{Name: "index", Type: TypeInt, Description: "image index (mutually exclusive with name)"},
			{Name: "name", Type: TypeString, Description: "image name (mutually exclusive with index)"},
			{Name: "force", Type: TypeBool, Description: "clear a non-empty mount directory first"},
		},
		Check: func(p Params, add AddIssue) {
			if p.Has("index") && p.Has("name") {
				add(CodeSemantic, "name", "index and name are mutually exclusive", false)
			}
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.MountADTWimFileOptions{
				ImagePath: p.StringOr("imagePath", ""),
				Path:      p.StringOr("path", ""),
				Index:     p.IntOr("index", 0),
				Name:      p.StringOr("name", ""),
				Force:     p.BoolOr("force", false),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.MountADTWimFile(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "wim.dismount", Summary: "Dismount a WIM image",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine,
				Description: "the WIM mount directory"},
			{Name: "save", Type: TypeBool, Description: "commit changes on dismount (default discards)"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.DismountADTWimFileOptions{
				Path: p.StringOr("path", ""),
				Save: p.BoolOr("save", false),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.DismountADTWimFile(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "regsvr32.invoke", Summary: "Register or unregister a DLL with an explicit action",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "filePath", Type: TypeString, Required: true, PathRole: PathPackageFile,
				Description: "DLL path (relative to Files/)"},
			{Name: "action", Type: TypeString, Required: true, Enum: []string{"register", "unregister"},
				Description: "regsvr32 action"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.InvokeADTRegSvr32Options{
				FilePath: p.StringOr("filePath", ""),
				Action:   p.StringOr("action", ""),
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.InvokeADTRegSvr32(ctx, opts)
			}, nil
		},
	})
}
