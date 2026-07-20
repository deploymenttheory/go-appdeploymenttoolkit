package manifest

import (
	"context"
	"regexp"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

// msiCommonParams are the parameters shared by the msi.install/uninstall/
// repair steps.
func msiCommonParams(install bool) []ParamSpec {
	params := []ParamSpec{
		{Name: "path", Type: TypeString, Required: true, PathRole: PathPackageFile,
			Description: "MSI file (relative to Files/) or an installed product-code GUID"},
		{Name: "transforms", Type: TypeStringList, PathRole: PathPackageFile,
			Description: "MST transforms applied via TRANSFORMS="},
		{Name: "arguments", Type: TypeString, Description: "override the default msiexec parameters from config"},
		{Name: "additionalArguments", Type: TypeString, Description: "append to the msiexec parameters"},
		{Name: "loggingOptions", Type: TypeString, Description: `override config MSI.LoggingOptions (default "/L*V")`},
		{Name: "logFileName", Type: TypeString, Description: "override the derived MSI log file name"},
		{Name: "secureArguments", Type: TypeBool, Description: "keep the composed msiexec arguments out of the log"},
		{Name: "successExitCodes", Type: TypeIntList, Description: "override the exit codes considered successful"},
		{Name: "rebootExitCodes", Type: TypeIntList, Description: "override the exit codes flagging a reboot"},
	}
	if install {
		params = append(params, ParamSpec{
			Name: "skipInstalledCheck", Type: TypeBool,
			Description: "skip the already-installed product check",
		})
	}
	return params
}

// bindMsi builds the Bind func for a fixed msiexec action.
func bindMsi(action string) func(p Params) (adt.PhaseFunc, error) {
	return func(p Params) (adt.PhaseFunc, error) {
		opts := msiOptionsFromParams(action, p)
		return func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcess(ctx, opts)
			return err
		}, nil
	}
}

// msiOptionsFromParams maps the shared msi params onto the option struct.
func msiOptionsFromParams(action string, p Params) adt.StartADTMsiProcessOptions {
	opts := adt.StartADTMsiProcessOptions{
		Action:                 action,
		Path:                   p.StringOr("path", ""),
		ArgumentList:           p.StringOr("arguments", ""),
		AdditionalArgumentList: p.StringOr("additionalArguments", ""),
		LoggingOptions:         p.StringOr("loggingOptions", ""),
		LogFileName:            p.StringOr("logFileName", ""),
		SecureArgumentList:     p.BoolOr("secureArguments", false),
	}
	if v, ok := p.StringList("transforms"); ok {
		opts.Transforms = v
	}
	if v, ok := p.IntList("successExitCodes"); ok {
		opts.SuccessExitCodes = v
	}
	if v, ok := p.IntList("rebootExitCodes"); ok {
		opts.RebootExitCodes = v
	}
	opts.SkipMSIAlreadyInstalledCheck = p.BoolOr("skipInstalledCheck", false)
	return opts
}

// checkMsiArguments warns when both argument params are set.
func checkMsiArguments(p Params, add AddIssue) {
	if p.Has("arguments") && p.Has("additionalArguments") {
		add(CodeSemantic, "additionalArguments",
			"additionalArguments appends to the overridden arguments — usually only one is intended", true)
	}
}

// guidRe matches product-code GUIDs (validation skips the file check for
// them).
var guidRe = regexp.MustCompile(`^\{?[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\}?$`)

// asUserParams extends a param table with the AsUser target selection.
func asUserParams(base []ParamSpec) []ParamSpec {
	return append(append([]ParamSpec{}, base...),
		ParamSpec{Name: "userName", Type: TypeString, Description: `target logged-on user ("user" or "DOMAIN\\user"); empty targets the first active session`},
		ParamSpec{Name: "allUsers", Type: TypeBool, Description: "run in every logged-on user session"},
	)
}

func init() {
	register(StepSpec{
		Name: "msi.install", Summary: "Install an MSI (or repair by product code) via msiexec",
		Platforms: []Platform{PlatformWindows},
		Params:    msiCommonParams(true),
		Check:     checkMsiArguments,
		Bind:      bindMsi("Install"),
	})
	register(StepSpec{
		Name: "msi.uninstall", Summary: "Uninstall an MSI by file or product code",
		Platforms: []Platform{PlatformWindows},
		Params:    msiCommonParams(false),
		Check:     checkMsiArguments,
		Bind:      bindMsi("Uninstall"),
	})
	register(StepSpec{
		Name: "msi.repair", Summary: "Repair an installed MSI product",
		Platforms: []Platform{PlatformWindows},
		Params:    msiCommonParams(false),
		Check:     checkMsiArguments,
		Bind:      bindMsi("Repair"),
	})
	register(StepSpec{
		Name: "msi.patch", Summary: "Apply an MSP patch",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "path", Type: TypeString, Required: true, PathRole: PathPackageFile,
				Description: "MSP patch file (relative to Files/)"},
			{Name: "additionalArguments", Type: TypeString, Description: "append to the msiexec parameters"},
			{Name: "loggingOptions", Type: TypeString, Description: "override config MSI.LoggingOptions"},
			{Name: "logFileName", Type: TypeString, Description: "override the derived log file name"},
			{Name: "secureArguments", Type: TypeBool, Description: "keep the composed arguments out of the log"},
			{Name: "successExitCodes", Type: TypeIntList, Description: "override the success exit codes"},
			{Name: "rebootExitCodes", Type: TypeIntList, Description: "override the reboot exit codes"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.StartADTMspProcessOptions{
				Path:                   p.StringOr("path", ""),
				AdditionalArgumentList: p.StringOr("additionalArguments", ""),
				LoggingOptions:         p.StringOr("loggingOptions", ""),
				LogFileName:            p.StringOr("logFileName", ""),
				SecureArgumentList:     p.BoolOr("secureArguments", false),
			}
			if v, ok := p.IntList("successExitCodes"); ok {
				opts.SuccessExitCodes = v
			}
			if v, ok := p.IntList("rebootExitCodes"); ok {
				opts.RebootExitCodes = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				_, err := adt.StartADTMspProcess(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "app.uninstall", Summary: "Uninstall installed applications by name or product code",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "name", Type: TypeStringList, Description: "display-name filters"},
			{Name: "nameMatch", Type: TypeString, Enum: []string{"contains", "exact", "wildcard", "regex"},
				Description: "how name filters match (default contains)"},
			{Name: "productCode", Type: TypeStringList, Description: "MSI product-code filters"},
			{Name: "applicationType", Type: TypeString, Enum: []string{"all", "msi", "exe"},
				Description: "restrict to MSI or EXE applications (default all)"},
			{Name: "arguments", Type: TypeString, Description: "override the uninstall arguments"},
			{Name: "additionalArguments", Type: TypeString, Description: "append to the uninstall arguments"},
		},
		Check: func(p Params, add AddIssue) {
			if !p.Has("name") && !p.Has("productCode") {
				add(CodeSemantic, "", "app.uninstall requires name or productCode", false)
			}
			if match, _ := p.String("nameMatch"); match == "regex" {
				if names, ok := p.StringList("name"); ok {
					for _, n := range names {
						if _, err := regexp.Compile(n); err != nil {
							add(CodeSemantic, "name", "invalid regular expression "+n, false)
							break
						}
					}
				}
			}
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			opts := adt.UninstallADTApplicationOptions{
				NameMatch:              p.StringOr("nameMatch", ""),
				ApplicationType:        p.StringOr("applicationType", ""),
				ArgumentList:           p.StringOr("arguments", ""),
				AdditionalArgumentList: p.StringOr("additionalArguments", ""),
			}
			if v, ok := p.StringList("name"); ok {
				opts.Name = v
			}
			if v, ok := p.StringList("productCode"); ok {
				opts.ProductCode = v
			}
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.UninstallADTApplication(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "msu.install", Summary: "Install Microsoft update packages (.msu) from a directory",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "directory", Type: TypeString, Required: true, PathRole: PathPackageFile,
				Description: "directory of .msu files (relative to Files/) or a single .msu"},
		},
		Bind: func(p Params) (adt.PhaseFunc, error) {
			dir := p.StringOr("directory", "")
			return func(ctx context.Context, s *adt.DeploymentSession) error {
				return adt.InstallADTMSUpdates(ctx, adt.InstallADTMSUpdatesOptions{Directory: dir})
			}, nil
		},
	})

	// AsUser variants share the msi param surface plus target selection.
	register(StepSpec{
		Name: "msi.installAsUser", Summary: "Install an MSI inside a logged-on user's session",
		Platforms: []Platform{PlatformWindows},
		Params:    asUserParams(msiCommonParams(true)),
		Check:     checkMsiArguments,
		Bind:      bindMsiAsUser("Install"),
	})
	register(StepSpec{
		Name: "msi.uninstallAsUser", Summary: "Uninstall an MSI inside a logged-on user's session",
		Platforms: []Platform{PlatformWindows},
		Params:    asUserParams(msiCommonParams(false)),
		Check:     checkMsiArguments,
		Bind:      bindMsiAsUser("Uninstall"),
	})
}

// bindMsiAsUser builds the Bind func for the msi *AsUser variants.
func bindMsiAsUser(action string) func(p Params) (adt.PhaseFunc, error) {
	return func(p Params) (adt.PhaseFunc, error) {
		opts := adt.StartADTMsiProcessAsUserOptions{
			StartADTMsiProcessOptions: msiOptionsFromParams(action, p),
			UserName:                  p.StringOr("userName", ""),
			AllUsers:                  p.BoolOr("allUsers", false),
		}
		return func(ctx context.Context, s *adt.DeploymentSession) error {
			_, err := adt.StartADTMsiProcessAsUser(ctx, opts)
			return err
		}, nil
	}
}
