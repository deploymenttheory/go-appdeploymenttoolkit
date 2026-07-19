// Package adt is a Go port of PSAppDeployToolkit (PSADT) v4: a toolkit for
// building Windows application deployments with session management, CMTrace
// logging, localized user dialogs, deferral, MSI helpers and system utilities.
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
// # Authoring a deployment
//
// A deployment is a small Go program describing its phases, the analogue of
// PSADT's Invoke-AppDeployToolkit.ps1:
//
//	func main() {
//	    (&adt.Deployment{
//	        Session: adt.SessionOptions{
//	            AppVendor: "VideoLAN", AppName: "VLC media player", AppVersion: "3.0.23",
//	        },
//	        Install: func(ctx context.Context, s *adt.DeploymentSession) error {
//	            _, err := adt.StartADTProcess(ctx, adt.StartADTProcessOptions{
//	                FilePath: "vlc-3.0.23-win64.exe", ArgumentList: "/S",
//	            })
//	            return err
//	        },
//	    }).Run(context.Background())
//	}
//
// Built binaries accept the PSADT frontend flags -DeploymentType and
// -DeployMode, e.g. `myapp.exe -DeploymentType Uninstall -DeployMode Silent`.
//
// The toolkit is Windows-only at runtime; it cross-compiles from any
// platform with GOOS=windows.
package adt
