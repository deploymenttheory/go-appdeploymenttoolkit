# go-appdeploymenttoolkit

A Go port of [PSAppDeployToolkit](https://psappdeploytoolkit.com) (PSADT) v4 — a
toolkit for building Windows application deployments. It provides deployment
session management, CMTrace logging, localized user dialogs, deferral logic,
MSI helpers and a broad set of Windows system utilities, as an importable Go
SDK plus a CLI runner.

> **Windows-only at runtime.** The toolkit targets `GOOS=windows`
> (amd64/arm64). It cross-compiles and unit-tests its portable logic on any
> platform; the syscall layers only execute on Windows.

## Function parity

Every exported function is a 1:1 port of the PSADT PowerShell function of the
same name with the hyphens removed, so existing deployment scripts translate
mechanically:

| PowerShell (PSADT) | Go (`adt`) |
| --- | --- |
| `Open-ADTSession` | `adt.OpenADTSession` |
| `Start-ADTMsiProcess` | `adt.StartADTMsiProcess` |
| `Show-ADTInstallationWelcome` | `adt.ShowADTInstallationWelcome` |
| `Copy-ADTFile` | `adt.CopyADTFile` |
| `Set-ADTRegistryKey` | `adt.SetADTRegistryKey` |

PowerShell-runtime plumbing (`Initialize-ADTFunction`, `New-ADTErrorRecord`, …)
has no Go counterpart; the `*-ADTModuleCallback` family is replaced by
`SessionHooks`, and `New-ADTTemplate` is provided by `adt new`.

## Authoring a deployment

A deployment is a small Go program describing its phases — the analogue of
PSADT's `Invoke-AppDeployToolkit.ps1`:

```go
package main

import (
    "context"

    "github.com/deploymenttheory/go-appdeploymenttoolkit/adt"
)

func main() {
    (&adt.Deployment{
        Session: adt.SessionOptions{
            AppVendor:  "VideoLAN",
            AppName:    "VLC media player",
            AppVersion: "3.0.23",
            AppProcessesToClose: []adt.ProcessObject{{Name: "vlc", Description: "VLC media player"}},
        },
        PreInstall: func(ctx context.Context, s *adt.DeploymentSession) error {
            _, err := adt.ShowADTInstallationWelcome(ctx, adt.ShowADTInstallationWelcomeOptions{
                CloseProcesses: s.Options().AppProcessesToClose, AllowDefer: true, DeferTimes: 3,
            })
            return err
        },
        Install: func(ctx context.Context, s *adt.DeploymentSession) error {
            _, err := adt.StartADTMsiProcess(ctx, adt.StartADTMsiProcessOptions{
                Action: "Install", Path: "vlc.msi",
            })
            return err
        },
    }).Run(context.Background())
}
```

Build it for Windows and run it with the standard PSADT frontend flags:

```sh
GOOS=windows go build -o Invoke-AppDeployToolkit.exe
Invoke-AppDeployToolkit.exe -DeploymentType Install -DeployMode Interactive
```

## Deployment manifests (YAML workflows)

Deployments can be defined as data instead of code: a `deployment.yaml` at the
package root composes steps from a typed catalog and is validated and executed
by the `adt` binary — locally or in a pipeline (validation is fully portable,
so CI can gate manifests without a Windows runner):

```yaml
apiVersion: v0.1.0-alpha
kind: Deployment
session:
  appVendor: Contoso
  appName: Widget
  appVersion: "1.2.3"
  closeProcesses: [{name: widget}]
phases:
  preInstall:
    - uses: dialog.welcome
      with: {allowDefer: true, deferTimes: 3, promptToSave: true}
  install:
    - uses: msi.install
      with: {path: Widget.msi, transforms: [Widget.mst]}
  postInstall:
    - uses: registry.set
      with: {key: 'HKLM:\SOFTWARE\Contoso\Widget', name: Installed, value: 1, type: dword}
```

- `adt validate <package-dir>` — layered validation (strict schema with
  did-you-mean suggestions, cross-field semantics, `Files/` existence,
  `--target` platform support) with gcc-style positions and `--json` output.
- `adt steps` — the step catalog (`--json` for tooling).
- `adt schema` — the format's JSON Schema (draft 2020-12), generated from the
  registry and checked in as `manifest/winadt.schema-v0.1.0-alpha.json`
  (semver-versioned artifact); emit it beside a package and reference it with
  a `yaml-language-server` modeline for editor autocomplete.
- The `manifest` package exposes the schema, catalog, validator and compiler
  programmatically.

## CLI

The `adt` command is the analogue of `Invoke-AppDeployToolkit.exe`:

- `adt run <package-dir>` — run a deployment package: a `deployment.yaml`
  manifest when present, else the **zero-config** MSI flow (single MSI under
  `Files/`, with MST/MSP auto-discovery) with no author code.
- `adt validate <package-dir>` / `adt steps` — manifest validation and the
  step catalog (see above).
- `adt new <dir> --name MyApp` — scaffold a package (`Files/`, `SupportFiles/`,
  `Config/`, `Strings/`, `Assets/`, a starter `deployment.yaml`, a `go.mod`
  and a deployment `main.go`).

```sh
go install github.com/deploymenttheory/go-appdeploymenttoolkit/cmd/adt@latest
adt validate ./MyPackage
adt run ./MyPackage --deployment-type Install --deploy-mode Silent
```

## Configuration & localization

- Package config lives in `Config/config.yaml`, mirroring PSADT's
  `config.psd1` section structure (Assets, MSI, Toolkit, UI); embedded defaults
  are used when it is absent.
- String tables ship for all 26 PSADT cultures plus English; override via
  `Strings/strings.yaml`.

## UI

Interactive dialogs render through Microsoft Edge WebView2 (via the CGo-free
`jchv/go-webview2`), with a native `MessageBox`/`TaskDialog` fallback when the
WebView2 runtime is absent. Deployments running as SYSTEM show dialogs in the
interactive user session via a same-binary client re-exec over anonymous pipes.

## Package layout

```
adt/          public SDK — all ~169 ported functions (one file per category)
internal/     domain implementations behind portable seams
cmd/adt/      CLI runner (run / new / client)
examples/     runnable deployment programs
tools/        the psd1→YAML converter for config and string tables
```

## Status

All five porting phases are complete: core session engine, system domains
(registry, filesystem, INI, process, MSI, services, shortcuts, users), UI and
cross-session client-server, CLI runner, and the long tail. See
[`docs/windows-smoke.md`](docs/windows-smoke.md) for the manual Windows
verification checklist (the syscall layers are cross-compiled and linted here
but only execute on Windows).

## Supporting libraries

- [`go-bindings-win32`](https://github.com/deploymenttheory/go-bindings-win32) —
  generated Win32 API bindings (MSI, WTS, registry, shell, tasks, …).
- [`go-bindings-wmi`](https://github.com/deploymenttheory/go-bindings-wmi) —
  typed WMI runtime (ConfigMgr `TriggerSchedule`, `Win32_QuickFixEngineering`).

## License

See [LICENSE](LICENSE).
