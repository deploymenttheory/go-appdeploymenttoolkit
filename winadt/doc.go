// Package winadt is the Windows SDK of go-appdeploymenttoolkit: a Go port
// of PSAppDeployToolkit (PSADT) v4 layered over the shared deploy engine.
// It provides the ~170 Windows domain functions (MSI helpers, registry,
// dialogs, services, processes, ...) plus PSADT-parity aliases for the
// engine's session/deployment types.
//
// # Function parity
//
// Every exported function is a 1:1 port of the PSADT PowerShell function of
// the same name with the hyphens removed — Start-ADTMsiProcess becomes
// [StartADTMsiProcess], Copy-ADTFile becomes [CopyADTFile] — so existing
// PSADT deployment scripts translate mechanically. PowerShell-runtime
// plumbing functions (Initialize-ADTFunction, New-ADTErrorRecord, ...) have
// no Go counterpart; the *-ADTModuleCallback family is replaced by
// [SessionHooks]; New-ADTTemplate is provided by the `adt new` CLI command.
//
// # Relationship to package deploy
//
// The session lifecycle, phase model, hooks, exit codes and logging live in
// the platform-neutral deploy package; winadt re-exports them under their
// PSADT names as identical type aliases ([DeploymentSession] = deploy.Session,
// [OpenADTSession] wraps deploy.Open, ...). A deployment may import either
// spelling; the macOS sibling (macadt) builds on the same engine.
//
// # Authoring a deployment
//
// A deployment is a small Go program describing its phases, the analogue of
// PSADT's Invoke-AppDeployToolkit.ps1:
//
//	func main() {
//	    (&winadt.Deployment{
//	        Session: winadt.SessionOptions{
//	            AppVendor: "VideoLAN", AppName: "VLC media player", AppVersion: "3.0.23",
//	        },
//	        Install: func(ctx context.Context, s *winadt.DeploymentSession) error {
//	            _, err := winadt.StartADTProcess(ctx, winadt.StartADTProcessOptions{
//	                FilePath: "vlc-3.0.23-win64.exe", ArgumentList: "/S",
//	            })
//	            return err
//	        },
//	    }).Run(context.Background())
//	}
//
// Built binaries accept the PSADT frontend flags -DeploymentType and
// -DeployMode, e.g. `myapp.exe -DeploymentType Uninstall -DeployMode Silent`.
// Deployments can also be declared as YAML manifests executed by the adt
// CLI; see the manifest package.
//
// This package's domain functions are Windows-only at runtime; the code
// cross-compiles from any platform with GOOS=windows.
package winadt
