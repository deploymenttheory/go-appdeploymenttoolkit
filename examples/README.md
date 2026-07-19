# Examples

Each subdirectory is a self-contained deployment program built on the `adt`
SDK — the Go analogue of an `Invoke-AppDeployToolkit.ps1`. They are compiled
per application:

```sh
cd examples/basic-msi
GOOS=windows go build -o Invoke-AppDeployToolkit.exe
Invoke-AppDeployToolkit.exe -DeploymentType Install -DeployMode Silent
```

Every built binary accepts the standard frontend flags: `-DeploymentType`
(`Install` | `Uninstall` | `Repair`), `-DeployMode` (`Auto` | `Interactive` |
`NonInteractive` | `Silent`) and `-SuppressRebootPassThru`.

| Example | Shows |
| --- | --- |
| [`basic-msi`](basic-msi) | A complete silent MSI deployment: `StartADTMsiProcess`, `CopyADTFile`, a registry deployment marker with `SetADTRegistryKey`, and full cleanup (`UninstallADTApplication` + `RemoveADTRegistryKey`) on uninstall. |
| [`interactive-install`](interactive-install) | An interactive EXE deployment: the welcome/close-apps prompt with deferral (`ShowADTInstallationWelcome`), a progress dialog, `StartADTProcess`, and a completion prompt. Ships a worked [`Config/config.yaml`](interactive-install/Config/config.yaml) overlay (branding, Fluent accent color, timeouts). |

## Example vs. deployment package

These are minimal single-file programs to illustrate the API. A real
deployment is a **package directory** — the layout `adt new` scaffolds:

```
MyPackage/
├── main.go            the deployment program (like these examples)
├── go.mod
├── Config/config.yaml optional config overrides (commented sample generated)
├── Strings/           optional localized string overrides
├── Assets/            logo/banner artwork
├── Files/             the installer payload (MSI/EXE/content)
└── SupportFiles/      extra files the deployment references
```

Scaffold one with:

```sh
adt new ./MyPackage --name "My Application"
```

Relative paths passed to functions like `StartADTMsiProcess` resolve against
the package's `Files/` directory, and support files against `SupportFiles/`,
just as in PSAppDeployToolkit.
