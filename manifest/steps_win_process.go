package manifest

import (
	"context"
	"strconv"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/winadt"
)

// processParams is the process.start parameter table (curated
// StartADTProcessOptions surface).
var processParams = []ParamSpec{
	{Name: "filePath", Type: TypeString, Required: true, PathRole: PathPackageFile,
		Description: "executable to launch; relative paths resolve against Files/"},
	{Name: "arguments", Type: TypeString, Description: "raw argument string"},
	{Name: "workingDirectory", Type: TypeString, PathRole: PathMachine, Description: "working directory override"},
	{Name: "windowStyle", Type: TypeString, Enum: []string{"normal", "hidden", "minimized", "maximized"},
		Description: "initial window disposition"},
	{Name: "createNoWindow", Type: TypeBool, Description: "suppress console window creation and capture output"},
	{Name: "useShellExecute", Type: TypeBool, Description: "launch without stream redirection"},
	{Name: "waitForMsiExec", Type: TypeBool, Description: "gate the launch on the Windows Installer mutex"},
	{Name: "msiExecWaitTime", Type: TypeDuration, Description: "bound the MSI mutex wait"},
	{Name: "successExitCodes", Type: TypeIntList, Description: "override the success exit codes"},
	{Name: "rebootExitCodes", Type: TypeIntList, Description: "override the reboot exit codes"},
	{Name: "ignoreExitCodes", Type: TypeStringList, Description: `exit codes to ignore; "*" ignores every code`},
	{Name: "timeout", Type: TypeDuration, Description: "bound the process runtime"},
	{Name: "noWait", Type: TypeBool, Description: "return immediately after launch"},
	{Name: "priorityClass", Type: TypeString,
		Enum:        []string{"idle", "belowNormal", "normal", "aboveNormal", "high", "realTime"},
		Description: "scheduling priority"},
	{Name: "verb", Type: TypeString, Description: `ShellExecute verb, e.g. "runas"`},
	{Name: "secureArguments", Type: TypeBool, Description: "keep arguments out of every log line"},
	{Name: "streamEncoding", Type: TypeString, Enum: []string{"utf-16le", "oem"},
		Description: "decode captured output with this encoding"},
	{Name: "noStreamLogging", Type: TypeBool, Description: "do not log captured StdOut/StdErr"},
	{Name: "expandEnvironmentVariables", Type: TypeBool, Description: "expand %VAR% in filePath/arguments"},
	{Name: "waitForChildProcesses", Type: TypeBool, Description: "wait for the whole spawned process tree"},
	{Name: "killChildProcessesWithParent", Type: TypeBool, Description: "terminate surviving children with the root process"},
	{Name: "noTerminateOnTimeout", Type: TypeBool, Description: "leave the process running when the timeout elapses"},
	{Name: "timeoutAction", Type: TypeString, Enum: []string{"error", "continue"},
		Description: "on timeout: error (default) or continue without error"},
	{Name: "denyUserTermination", Type: TypeBool, Description: "deny the interactive user PROCESS_TERMINATE"},
}

// checkProcess applies process.start's cross-field rules.
func checkProcess(p Params, add AddIssue) {
	if (p.Has("timeoutAction") || p.Has("noTerminateOnTimeout")) && !p.Has("timeout") {
		add(CodeSemantic, "timeout", "timeoutAction/noTerminateOnTimeout have no effect without timeout", true)
	}
	if p.BoolOr("useShellExecute", false) && p.BoolOr("createNoWindow", false) {
		add(CodeSemantic, "useShellExecute", "useShellExecute disables the stream capture createNoWindow enables", true)
	}
	if codes, ok := p.StringList("ignoreExitCodes"); ok {
		for _, c := range codes {
			if c == "*" {
				continue
			}
			if _, err := strconv.Atoi(c); err != nil {
				add(CodeSemantic, "ignoreExitCodes", `entries must be integers or "*", got `+strconv.Quote(c), false)
				break
			}
		}
	}
}

// processOptionsFromParams maps the process params onto the option struct.
func processOptionsFromParams(p Params) winadt.StartADTProcessOptions {
	opts := winadt.StartADTProcessOptions{
		FilePath:                     p.StringOr("filePath", ""),
		ArgumentList:                 p.StringOr("arguments", ""),
		WorkingDirectory:             p.StringOr("workingDirectory", ""),
		WindowStyle:                  p.StringOr("windowStyle", ""),
		CreateNoWindow:               p.BoolOr("createNoWindow", false),
		UseShellExecute:              p.BoolOr("useShellExecute", false),
		WaitForMsiExec:               p.BoolOr("waitForMsiExec", false),
		NoWait:                       p.BoolOr("noWait", false),
		PriorityClass:                p.StringOr("priorityClass", ""),
		Verb:                         p.StringOr("verb", ""),
		SecureArgumentList:           p.BoolOr("secureArguments", false),
		StreamEncoding:               p.StringOr("streamEncoding", ""),
		NoStreamLogging:              p.BoolOr("noStreamLogging", false),
		ExpandEnvironmentVariables:   p.BoolOr("expandEnvironmentVariables", false),
		WaitForChildProcesses:        p.BoolOr("waitForChildProcesses", false),
		KillChildProcessesWithParent: p.BoolOr("killChildProcessesWithParent", false),
		NoTerminateOnTimeout:         p.BoolOr("noTerminateOnTimeout", false),
		TimeoutAction:                p.StringOr("timeoutAction", ""),
		DenyUserTermination:          p.BoolOr("denyUserTermination", false),
	}
	if v, ok := p.Duration("msiExecWaitTime"); ok {
		opts.MsiExecWaitTime = v
	}
	if v, ok := p.Duration("timeout"); ok {
		opts.Timeout = v
	}
	if v, ok := p.IntList("successExitCodes"); ok {
		opts.SuccessExitCodes = v
	}
	if v, ok := p.IntList("rebootExitCodes"); ok {
		opts.RebootExitCodes = v
	}
	if v, ok := p.StringList("ignoreExitCodes"); ok {
		opts.IgnoreExitCodes = v
	}
	return opts
}

func init() {
	register(StepSpec{
		Name: "process.start", Summary: "Launch a process and classify its exit code",
		Platforms: []Platform{PlatformWindows},
		Params:    processParams,
		Check:     checkProcess,
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := processOptionsFromParams(p)
			return func(ctx context.Context, s *deploy.Session) error {
				_, err := winadt.StartADTProcess(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "process.startAsUser", Summary: "Launch a process inside a logged-on user's session",
		Platforms: []Platform{PlatformWindows},
		Params: append(asUserParams(processParams),
			ParamSpec{Name: "useLinkedAdminToken", Type: TypeBool, Description: "require the user's linked (elevated) admin token"},
			ParamSpec{Name: "useHighestAvailableToken", Type: TypeBool, Description: "use the linked admin token when available, else the base token"},
			ParamSpec{Name: "useUnelevatedToken", Type: TypeBool, Description: "force the limited token even for admin users"},
			ParamSpec{Name: "inheritEnvironmentVariables", Type: TypeBool, Description: "pass the calling process's environment"},
		),
		Check: func(p Params, add AddIssue) {
			checkProcess(p, add)
			set := 0
			for _, name := range []string{"useLinkedAdminToken", "useHighestAvailableToken", "useUnelevatedToken"} {
				if p.BoolOr(name, false) {
					set++
				}
			}
			if set > 1 {
				add(CodeSemantic, "useLinkedAdminToken",
					"useLinkedAdminToken, useHighestAvailableToken and useUnelevatedToken are mutually exclusive", false)
			}
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.StartADTProcessAsUserOptions{
				StartADTProcessOptions:      processOptionsFromParams(p),
				UserName:                    p.StringOr("userName", ""),
				AllUsers:                    p.BoolOr("allUsers", false),
				UseLinkedAdminToken:         p.BoolOr("useLinkedAdminToken", false),
				UseHighestAvailableToken:    p.BoolOr("useHighestAvailableToken", false),
				UseUnelevatedToken:          p.BoolOr("useUnelevatedToken", false),
				InheritEnvironmentVariables: p.BoolOr("inheritEnvironmentVariables", false),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				_, err := winadt.StartADTProcessAsUser(ctx, opts)
				return err
			}, nil
		},
	})
	register(StepSpec{
		Name: "process.blockExecution", Summary: "Block the named applications from starting (IFEO)",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "processes", Type: TypeStringList, Required: true,
				Description: "process names (without extension) to block"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			names, _ := p.StringList("processes")
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.BlockADTAppExecution(ctx, names)
			}, nil
		},
	})
	register(StepSpec{
		Name: "process.unblockExecution", Summary: "Lift application-execution blocks installed by this toolkit",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.UnblockADTAppExecution(ctx)
			}, nil
		},
	})
	register(StepSpec{
		Name: "process.sendKeys", Summary: "Send a key sequence to a window",
		Platforms: []Platform{PlatformWindows},
		Params: []ParamSpec{
			{Name: "windowTitle", Type: TypeString, Required: true, Description: "substring match for the target window title"},
			{Name: "keys", Type: TypeString, Required: true, Description: "key sequence to type"},
			{Name: "waitSeconds", Type: TypeInt, Description: "seconds to wait for the window"},
		},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			opts := winadt.SendADTKeysOptions{
				WindowTitle: p.StringOr("windowTitle", ""),
				Keys:        p.StringOr("keys", ""),
				WaitSeconds: p.IntOr("waitSeconds", 0),
			}
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.SendADTKeys(ctx, opts)
			}, nil
		},
	})
	register(StepSpec{
		Name: "desktop.update", Summary: "Refresh the desktop and broadcast environment changes",
		Platforms: []Platform{PlatformWindows},
		Bind: func(p Params) (deploy.PhaseFunc, error) {
			return func(ctx context.Context, s *deploy.Session) error {
				return winadt.UpdateADTDesktop(ctx)
			}, nil
		},
	})
}
