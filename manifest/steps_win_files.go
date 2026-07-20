package manifest

import (
	"context"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/fsops"
)

// permissionFromString maps the manifest enum onto fsops.Permission (the
// values are the canonical names themselves).
func permissionFromString(s string) fsops.Permission { return fsops.Permission(s) }

// accessActionFromString maps grant/deny/remove onto fsops.AccessAction.
func accessActionFromString(s string) fsops.AccessAction {
	switch strings.ToLower(s) {
	case "deny":
		return fsops.ActionDeny
	case "remove":
		return fsops.ActionRemove
	default:
		return fsops.ActionGrant
	}
}

// inheritanceFromString maps none/object/container/both onto
// fsops.InheritanceScope.
func inheritanceFromString(s string) fsops.InheritanceScope {
	switch strings.ToLower(s) {
	case "object":
		return fsops.InheritObject
	case "container":
		return fsops.InheritContainer
	case "both":
		return fsops.InheritObject | fsops.InheritContainer
	default:
		return fsops.InheritNone
	}
}

// copyParams is shared by file.copy and file.copyToUserProfiles.
var copyParams = []ParamSpec{
	{Name: "path", Type: TypeStringList, Required: true, PathRole: PathPackageFile,
		Description: "source files/folders (relative paths resolve against Files/)"},
	{Name: "destination", Type: TypeString, Required: true, PathRole: PathMachine,
		Description: "destination path on the target machine"},
	{Name: "recurse", Type: TypeBool, Description: "copy folder trees recursively"},
	{Name: "flatten", Type: TypeBool, Description: "copy all files into the destination root"},
	{Name: "continueOnError", Type: TypeBool, Description: "keep copying when individual items fail"},
	{Name: "fileCopyMode", Type: TypeString, Enum: []string{"native", "robocopy"},
		Description: "copy engine (default from config Toolkit.FileCopyMode)"},
}

// copyOptionsFromParams maps the shared copy params onto the option struct.
func copyOptionsFromParams(p Params) winadt.CopyADTFileOptions {
	opts := winadt.CopyADTFileOptions{
		Destination:             p.StringOr("destination", ""),
		Recurse:                 p.BoolOr("recurse", false),
		Flatten:                 p.BoolOr("flatten", false),
		ContinueFileCopyOnError: p.BoolOr("continueOnError", false),
		FileCopyMode:            p.StringOr("fileCopyMode", ""),
	}
	if v, ok := p.StringList("path"); ok {
		opts.Path = v
	}
	return opts
}

func init() {
	register(StepSpec{
		Name: "file.copy", Summary: "Copy files or folders to the target machine",
		Platforms: []Platform{PlatformWindows},
		Params:    copyParams,
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := copyOptionsFromParams(p)
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.CopyADTFile(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "file.remove", Summary: "Remove files or folders from the target machine",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeStringList, Required: true, PathRole: PathMachine,
				Description: "paths to remove (globs allowed)"},
			{Name: "recurse", Type: TypeBool, Description: "remove folder trees recursively"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.RemoveADTFileOptions{Recurse: p.BoolOr("recurse", false)}
			if v, ok := p.StringList("path"); ok {
				opts.Path = v
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTFile(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "file.copyToUserProfiles", Summary: "Copy items into every user profile",
		Platforms: []Platform{PlatformWindows},
		Params:    copyParams,
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			base := copyOptionsFromParams(p)
			opts := winadt.CopyADTFileToUserProfilesOptions{
				Path:                    base.Path,
				Destination:             base.Destination,
				Recurse:                 base.Recurse,
				Flatten:                 base.Flatten,
				ContinueFileCopyOnError: base.ContinueFileCopyOnError,
				FileCopyMode:            base.FileCopyMode,
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.CopyADTFileToUserProfiles(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "file.removeFromUserProfiles", Summary: "Remove items from every user profile",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeStringList, Required: true, PathRole: PathMachine,
				Description: "profile-relative paths to remove"},
			{Name: "recurse", Type: TypeBool, Description: "remove folder trees recursively"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.RemoveADTFileFromUserProfilesOptions{Recurse: p.BoolOr("recurse", false)}
			if v, ok := p.StringList("path"); ok {
				opts.Path = v
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTFileFromUserProfiles(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "folder.create", Summary: "Create a folder (and parents)",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine, Description: "folder to create"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			path := p.StringOr("path", "")
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.NewADTFolder(ctx, path)
			}, nil
		},
	})
	register(StepSpec{
		Name: "folder.remove", Summary: "Remove a folder and its contents",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine, Description: "folder to remove"},
			{Name: "disableRecursion", Type: TypeBool, Description: "only remove the folder's own files"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.RemoveADTFolderOptions{
				Path:             p.StringOr("path", ""),
				DisableRecursion: p.BoolOr("disableRecursion", false),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTFolder(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "cache.copy", Summary: "Copy the package content to the local cache",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Description: "cache destination (default from config Toolkit.CachePath)"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			path := p.StringOr("path", "")
			return func(ctx context.Context, s *deploy.Session) error {
				_, err := winadt.CopyADTContentToCache(ctx, path)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "cache.remove", Summary: "Remove the cached package content",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTContentFromCache(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "zip.create", Summary: "Create a zip archive",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeStringList, Required: true, PathRole: PathMachine,
				Description: "files/folders to archive"},
			{Name: "destination", Type: TypeString, Required: true, PathRole: PathMachine,
				Description: "the .zip file to create"},
			{Name: "removeSource", Type: TypeBool, Description: "delete the sources after archiving"},
			{Name: "overwrite", Type: TypeBool, Description: "replace an existing archive"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.NewADTZipFileOptions{
				DestinationPath:            p.StringOr("destination", ""),
				RemoveSourceAfterArchiving: p.BoolOr("removeSource", false),
				Overwrite:                  p.BoolOr("overwrite", false),
			}
			if v, ok := p.StringList("path"); ok {
				opts.LiteralPath = v
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.NewADTZipFile(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "ini.set", Summary: "Write an INI value",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "filePath", Type: TypeString, Required: true, PathRole: PathMachine, Description: "INI file"},
			{Name: "section", Type: TypeString, Required: true, Description: "section name"},
			{Name: "key", Type: TypeString, Required: true, Description: "key name"},
			{Name: "value", Type: TypeString, Description: "value (empty writes Key=)"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.SetADTIniValueOptions{
				FilePath: p.StringOr("filePath", ""),
				Section:  p.StringOr("section", ""),
				Key:      p.StringOr("key", ""),
				Value:    p.StringOr("value", ""),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.SetADTIniValue(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "ini.remove", Summary: "Remove an INI key",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "filePath", Type: TypeString, Required: true, PathRole: PathMachine, Description: "INI file"},
			{Name: "section", Type: TypeString, Required: true, Description: "section name"},
			{Name: "key", Type: TypeString, Required: true, Description: "key to remove"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.IniValueOptions{
				FilePath: p.StringOr("filePath", ""),
				Section:  p.StringOr("section", ""),
				Key:      p.StringOr("key", ""),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTIniValue(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "ini.removeSection", Summary: "Remove a whole INI section",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "filePath", Type: TypeString, Required: true, PathRole: PathMachine, Description: "INI file"},
			{Name: "section", Type: TypeString, Required: true, Description: "section to remove"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.IniSectionOptions{
				FilePath: p.StringOr("filePath", ""),
				Section:  p.StringOr("section", ""),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.RemoveADTIniSection(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "permission.set", Summary: "Grant, deny or remove NTFS/registry permissions",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Required: true, PathRole: PathMachine,
				Description: "file, folder or registry key"},
			{Name: "user", Type: TypeString, Required: true, Description: `NTAccount ("DOMAIN\\User") or SID`},
			{Name: "action", Type: TypeString, Enum: []string{"grant", "deny", "remove"},
				Description: "permission action (default grant)"},
			{Name: "permission", Type: TypeString,
				Enum:        []string{"FullControl", "Modify", "ReadAndExecute", "Read", "Write"},
				Description: "simplified permission set"},
			{Name: "inheritance", Type: TypeString, Enum: []string{"none", "object", "container", "both"},
				Description: "how the ACE propagates to children (default none)"},
			{Name: "enableInheritance", Type: TypeBool, Description: "re-enable ACL inheritance from the parent"},
			{Name: "disableInheritance", Type: TypeBool, Description: "protect the ACL from parent inheritance"},
		},
		Check: func(p Params, add AddIssue) {
			if !p.BoolOr("enableInheritance", false) && !p.Has("permission") {
				add(CodeSemantic, "permission", "permission is required unless enableInheritance is set", false)
			}
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.SetADTItemPermissionOptions{
				Path:               p.StringOr("path", ""),
				User:               p.StringOr("user", ""),
				Permission:         permissionFromString(p.StringOr("permission", "")),
				Action:             accessActionFromString(p.StringOr("action", "")),
				Inheritance:        inheritanceFromString(p.StringOr("inheritance", "")),
				EnableInheritance:  p.BoolOr("enableInheritance", false),
				DisableInheritance: p.BoolOr("disableInheritance", false),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.SetADTItemPermission(ctx, opts)
			}, nil
		},
	})
}
