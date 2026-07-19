# Windows smoke-test checklist

The toolkit is developed and unit-tested on non-Windows hosts (portable logic
plus `GOOS=windows` cross-compilation). The Windows-only syscall layers are
thin marshaling code that cannot run in CI here. This checklist enumerates the
behaviors to verify on a real Windows host once one is available, grouped by
phase.

## Phase 1 — Core session

- [ ] `OpenADTSession` writes a CMTrace log under `%SystemRoot%\Logs\Software`
      (admin) or `%ProgramData%\Logs\Software` (non-admin); the header lines
      and divider render correctly in OneTrace/CMTrace.
- [ ] Auto deploy-mode resolves to Interactive with a console user logged on,
      Silent as SYSTEM with no active session.
- [ ] Deferral values round-trip with a real PowerShell PSADT deployment of the
      same `InstallName` (shared `HKLM\SOFTWARE\PSAppDeployToolkit\DeferHistory`).
- [ ] `CloseADTSession` classifies 0/3010/1641/other exit codes and suppresses
      reboot passthru when requested.

## Phase 2 — System domains

- [ ] Registry: Get/Set/Remove/Test round-trip incl. `Wow6432Node` redirection
      on 64-bit; `InvokeADTAllUsersRegistryAction` visits loaded HKU hives.
- [ ] Filesystem: `CopyADTFile` Native and Robocopy modes; `CopyADTFileToUserProfiles`
      writes into each real profile; `SetADTItemPermission` applies/removes ACEs.
- [ ] `GetADTFileVersion` reads a real signed binary's version resource.
- [ ] INI: read/write against a UTF-16LE INI preserves encoding and comments.
- [ ] Process: `StartADTProcess` waits, captures output, honors success/reboot/
      ignore exit codes; `StartADTProcessAsUser` launches in the console session;
      MSI mutex gate blocks while another install holds `Global\_MSIExecute`.
- [ ] Services: start/stop with dependents; start-mode get/set incl. delayed-auto.
- [ ] Shortcuts: `.lnk` create/read/update via IShellLinkW; hotkeys applied.
- [ ] Users: `GetADTLoggedOnUser`, `GetADTUserProfiles`, SID⇄NTAccount conversion.
- [ ] Environment variables: User/Machine writes broadcast `WM_SETTINGCHANGE`.
- [ ] MSI: `StartADTMsiProcess` install/uninstall/repair; `GetADTApplication` and
      `UninstallADTApplication` against real uninstall entries.

## Phase 5 — Long tail

- [ ] Fonts: `AddADTFont`/`RemoveADTFont` register in `%WINDIR%\Fonts` and take effect after `WM_FONTCHANGE`.
- [ ] `AddADTEdgeExtension`/`RemoveADTEdgeExtension` update the Edge `ExtensionSettings` policy JSON.
- [ ] `SetADTActiveSetup` writes the component key and executes the StubPath.
- [ ] `RegisterADTDll`/`UnregisterADTDll` via architecture-correct regsvr32.
- [ ] `Enable/DisableADTTerminalServerInstallMode` via `change user`.
- [ ] `UpdateADTGroupPolicy` runs gpupdate for computer and user.
- [ ] `MountADTWimFile`/`DismountADTWimFile` via dism.exe.
- [ ] `GetADTExecutableInfo` reads a signed binary's version resource.
- [ ] System-state probes: battery, network, PowerPoint slideshow, microphone,
      OOBE, ESP, pending-reboot aggregation, toast mode.
- [ ] `InstallADTMSUpdates` installs .msu packages; `TestADTMSUpdates` detects a
      KB via `Win32_QuickFixEngineering`.
- [ ] `InvokeADTSCCMTask` / `InstallADTSCCMSoftwareUpdates` trigger the
      `ROOT\CCM SMS_Client.TriggerSchedule` WMI method (requires the ConfigMgr
      client). Verify the schedule fires in the ConfigMgr control panel.

## Phase 3 — UI + client-server

- [ ] WebView2 dialogs render (welcome/close-apps, progress, restart, prompt);
      native TaskDialog fallback when the WebView2 runtime is absent.
- [ ] SYSTEM-context deployment shows dialogs in the interactive user session via
      the re-exec client over anonymous pipes.
- [ ] `BlockADTAppExecution` installs and cleanly removes the IFEO Debugger keys.
