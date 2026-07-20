package manifest

// This file defines the session: block schema — the curated camelCase mirror
// of adt.SessionOptions. Deliberately excluded fields: deploymentType (runtime
// --deployment-type selects the phase triplet), ScriptDirectory/DirFiles/
// DirSupportFiles (derived from the package directory), ConfigOverlayPath/
// StringsOverlayPath (convention: Config/config.yaml, Strings/strings.yaml),
// Hooks and AppScript* (Go-code-only).

// sessionParamSpecs is the session block's parameter table.
var sessionParamSpecs = []ParamSpec{
	{Name: "appVendor", Type: TypeString, Description: "application vendor"},
	{Name: "appName", Type: TypeString, Description: "application name"},
	{Name: "appVersion", Type: TypeString, Description: "application version"},
	{Name: "appArch", Type: TypeString, Description: "application architecture label (e.g. x64)"},
	{Name: "appLang", Type: TypeString, Description: `application language tag (default "EN")`},
	{Name: "appRevision", Type: TypeString, Description: `package revision (default "01")`},
	{Name: "installName", Type: TypeString, Description: "override the composed install name"},
	{Name: "installTitle", Type: TypeString, Description: "override the dialog title"},
	{
		Name: "deployMode", Type: TypeString,
		Enum:        []string{"auto", "interactive", "nonInteractive", "silent"},
		Description: "author-intended deploy mode; the runtime --deploy-mode flag wins",
	},
	{Name: "closeProcesses", Type: TypeProcessList, Description: "processes the welcome dialog offers to close (AppProcessesToClose)"},
	{Name: "successExitCodes", Type: TypeIntList, Description: "exit codes treated as success (default [0])"},
	{Name: "rebootExitCodes", Type: TypeIntList, Description: "exit codes flagging a reboot (default [1641, 3010])"},
	{Name: "logName", Type: TypeString, Description: "override the derived log file name"},
	{Name: "disableLogging", Type: TypeBool, Description: "disable the session log file"},
	{Name: "suppressRebootPassThru", Type: TypeBool, Description: "return 0 instead of a reboot exit code"},
	{Name: "terminalServerMode", Type: TypeBool, Description: "toggle RDS install mode around the deployment"},
	{Name: "requireAdmin", Type: TypeBool, Description: "fail fast unless running with administrative rights"},
	{Name: "languageOverride", Type: TypeString, Description: "UI language override (wins over config)"},
	{Name: "noOobeDetection", Type: TypeBool, Description: "skip OOBE/ESP checks during Auto deploy-mode resolution"},
	{Name: "noProcessDetection", Type: TypeBool, Description: "skip processes-to-close checks during Auto deploy-mode resolution"},
	{Name: "noSessionDetection", Type: TypeBool, Description: "skip session-0 checks during Auto deploy-mode resolution"},
	{Name: "processInteractivityDetection", Type: TypeBool, Description: "in session 0, require an interactive window station"},
}

// checkSession applies the session block's semantic rules.
func checkSession(p Params, add AddIssue) {
	if procs, ok := p.ProcessList("closeProcesses"); ok {
		for _, proc := range procs {
			if proc.Name == "" {
				add(CodeSemantic, "closeProcesses", "closeProcesses entries need a non-empty name", false)
				break
			}
		}
	}
}
