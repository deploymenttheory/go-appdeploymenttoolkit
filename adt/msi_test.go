package adt

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/config"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

// testMsiConfig returns an MSI config section matching the embedded defaults.
func testMsiConfig() config.MSI {
	return config.MSI{
		InstallParams:   "REBOOT=ReallySuppress /QB-!",
		SilentParams:    "REBOOT=ReallySuppress /QN",
		UninstallParams: "REBOOT=ReallySuppress /QN",
		LoggingOptions:  "/L*V",
		MutexWaitTime:   600,
	}
}

func TestBuildMsiArgumentsInstallDefaults(t *testing.T) {
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:  "Install",
		Product: `C:\Files\app.msi`,
		LogFile: `C:\Logs\app_Install.log`,
	}, testMsiConfig())
	require.NoError(t, err)
	assert.Equal(t,
		`/i "C:\Files\app.msi" REBOOT=ReallySuppress /QB-! /L*V "C:\Logs\app_Install.log"`,
		args)
}

func TestBuildMsiArgumentsSilentUsesSilentParams(t *testing.T) {
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:  "Install",
		Product: `C:\Files\app.msi`,
		Silent:  true,
	}, testMsiConfig())
	require.NoError(t, err)
	assert.Equal(t, `/i "C:\Files\app.msi" REBOOT=ReallySuppress /QN`, args)
}

func TestBuildMsiArgumentsUninstallProductCode(t *testing.T) {
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:  "Uninstall",
		Product: "{26923B43-4D38-484F-9B9E-DE460746276C}",
		LogFile: `C:\Logs\app_Uninstall.log`,
	}, testMsiConfig())
	require.NoError(t, err)
	assert.Equal(t,
		`/x {26923B43-4D38-484F-9B9E-DE460746276C} REBOOT=ReallySuppress /QN /L*V "C:\Logs\app_Uninstall.log"`,
		args)
}

func TestBuildMsiArgumentsTransformsAndOverrides(t *testing.T) {
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:       "Install",
		Product:      `C:\Files\app.msi`,
		Transforms:   []string{`C:\Files\a.mst`, `C:\Files\b.mst`},
		ArgumentList: "/QN ALLUSERS=1",
	}, testMsiConfig())
	require.NoError(t, err)
	assert.Equal(t,
		`/i "C:\Files\app.msi" TRANSFORMS="C:\Files\a.mst;C:\Files\b.mst" TRANSFORMSSECURE=1 /QN ALLUSERS=1`,
		args)
}

func TestBuildMsiArgumentsAdditionalAndLoggingOverride(t *testing.T) {
	args, err := buildMsiArguments(msiArgumentInputs{
		Action:                 "Install",
		Product:                `C:\Files\app.msi`,
		AdditionalArgumentList: "SOMEPROPERTY=TRUE",
		LoggingOptions:         "/L*V+",
		LogFile:                `C:\Logs\app_Install.log`,
	}, testMsiConfig())
	require.NoError(t, err)
	assert.Equal(t,
		`/i "C:\Files\app.msi" REBOOT=ReallySuppress /QB-! SOMEPROPERTY=TRUE /L*V+ "C:\Logs\app_Install.log"`,
		args)
}

func TestBuildMsiArgumentsActionSwitches(t *testing.T) {
	cfg := testMsiConfig()
	cases := map[string]string{
		"Patch":       `/p "C:\Files\patch.msp" REBOOT=ReallySuppress /QB-!`,
		"Repair":      `/fomus "C:\Files\app.msi" REBOOT=ReallySuppress /QB-!`,
		"ActiveSetup": `/fups "C:\Files\app.msi"`,
	}
	for action, want := range cases {
		product := `C:\Files\app.msi`
		if action == "Patch" {
			product = `C:\Files\patch.msp`
		}
		args, err := buildMsiArguments(msiArgumentInputs{Action: action, Product: product}, cfg)
		require.NoError(t, err, action)
		assert.Equal(t, want, args, action)
	}
	_, err := buildMsiArguments(msiArgumentInputs{Action: "Sideload", Product: "x"}, cfg)
	require.ErrorIs(t, err, ErrInvalidOption)
}

func TestCanonicalMsiAction(t *testing.T) {
	for input, want := range map[string]string{
		"":            "Install",
		"install":     "Install",
		"UNINSTALL":   "Uninstall",
		"repair":      "Repair",
		"patch":       "Patch",
		"activesetup": "ActiveSetup",
	} {
		got, err := canonicalMsiAction(input)
		require.NoError(t, err, input)
		assert.Equal(t, want, got, input)
	}
	_, err := canonicalMsiAction("Delete")
	require.ErrorIs(t, err, ErrInvalidOption)
}

func TestResolveMsiLogFile(t *testing.T) {
	cfg := &config.Config{}
	cfg.MSI.LogPath = `C:\Logs\MSI`
	cfg.MSI.LoggingOptions = "/L*V"

	got := resolveMsiLogFile(nil, cfg, "", `C:\Files\My App.msi`, "Install")
	assert.Equal(t, filepath.Join(`C:\Logs\MSI`, "MyApp_Install.log"), got)

	got = resolveMsiLogFile(nil, cfg, "custom.txt", `C:\Files\app.msi`, "Uninstall")
	assert.Equal(t, filepath.Join(`C:\Logs\MSI`, "custom_Uninstall.txt"), got)

	got = resolveMsiLogFile(nil, cfg, "", "{26923B43-4D38-484F-9B9E-DE460746276C}", "Uninstall")
	assert.Equal(t,
		filepath.Join(`C:\Logs\MSI`, "{26923B43-4D38-484F-9B9E-DE460746276C}_Uninstall.log"),
		got)

	// Log name already ending with the action is not suffixed again.
	got = resolveMsiLogFile(nil, cfg, "app_Install", `C:\Files\app.msi`, "Install")
	assert.Equal(t, filepath.Join(`C:\Logs\MSI`, "app_Install.log"), got)

	// No MSI log path falls back to the toolkit log path.
	fallback := &config.Config{}
	fallback.Toolkit.LogPath = `C:\Logs\Software`
	got = resolveMsiLogFile(nil, fallback, "", `C:\Files\app.msi`, "Install")
	assert.Equal(t, filepath.Join(`C:\Logs\Software`, "app_Install.log"), got)

	// No resolvable directory disables MSI logging.
	got = resolveMsiLogFile(nil, &config.Config{}, "", `C:\Files\app.msi`, "Install")
	assert.Empty(t, got)
}

func TestSplitUninstallCommand(t *testing.T) {
	file, args := splitUninstallCommand(`"C:\Program Files\App\uninstall.exe" /S /NORESTART`)
	assert.Equal(t, `C:\Program Files\App\uninstall.exe`, file)
	assert.Equal(t, "/S /NORESTART", args)

	file, args = splitUninstallCommand(`C:\App\remove.exe /quiet`)
	assert.Equal(t, `C:\App\remove.exe`, file)
	assert.Equal(t, "/quiet", args)

	file, args = splitUninstallCommand(`"C:\App\remove.exe"`)
	assert.Equal(t, `C:\App\remove.exe`, file)
	assert.Empty(t, args)

	file, args = splitUninstallCommand("remove.exe")
	assert.Equal(t, "remove.exe", file)
	assert.Empty(t, args)
}

func TestGetADTMsiExitCodeMessage(t *testing.T) {
	assert.Equal(t,
		"The user cancelled installation. (ERROR_INSTALL_USEREXIT)",
		GetADTMsiExitCodeMessage(1602))
	assert.Contains(t, GetADTMsiExitCodeMessage(1618), "Another installation is already in progress")
}

// seedUninstallApp writes one uninstall entry into the fake registry.
func seedUninstallApp(t *testing.T, fake *regkey.Fake, hive, key string, values map[string]regkey.Value) {
	t.Helper()
	path := `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\` + key
	require.NoError(t, fake.CreateKey(hive, path))
	for name, value := range values {
		require.NoError(t, fake.SetValue(hive, path, name, value))
	}
}

func regStr(s string) regkey.Value { return regkey.Value{Kind: regkey.KindString, Data: s} }

func TestGetADTApplication(t *testing.T) {
	fake := installFake(t)
	seedUninstallApp(t, fake, "HKLM", "{26923B43-4D38-484F-9B9E-DE460746276C}", map[string]regkey.Value{
		"DisplayName":      regStr("Adobe Acrobat Reader"),
		"DisplayVersion":   regStr("23.1.0"),
		"UninstallString":  regStr(`MsiExec.exe /X{26923B43-4D38-484F-9B9E-DE460746276C}`),
		"WindowsInstaller": regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)},
	})
	seedUninstallApp(t, fake, "HKLM", "SomeTool", map[string]regkey.Value{
		"DisplayName":     regStr("Some Tool"),
		"UninstallString": regStr(`"C:\Tools\uninstall.exe"`),
	})

	apps, err := GetADTApplication(context.Background(), GetADTApplicationOptions{Name: []string{"Acrobat"}})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "Adobe Acrobat Reader", apps[0].DisplayName)
	assert.Equal(t, "{26923B43-4D38-484F-9B9E-DE460746276C}", apps[0].ProductCode)
	assert.True(t, apps[0].WindowsInstaller)

	apps, err = GetADTApplication(context.Background(), GetADTApplicationOptions{ApplicationType: "EXE"})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "Some Tool", apps[0].DisplayName)

	apps, err = GetADTApplication(context.Background(), GetADTApplicationOptions{Name: []string{"Absent"}})
	require.NoError(t, err)
	assert.Empty(t, apps)
}

func TestUninstallADTApplicationRequiresFilter(t *testing.T) {
	installFake(t)
	err := UninstallADTApplication(context.Background(), UninstallADTApplicationOptions{})
	require.ErrorIs(t, err, ErrInvalidOption)
}

func TestUninstallADTApplicationNoMatches(t *testing.T) {
	installFake(t)
	err := UninstallADTApplication(context.Background(), UninstallADTApplicationOptions{
		Name: []string{"Definitely Not Installed"},
	})
	require.NoError(t, err)
}

func TestUninstallADTApplicationSkipsMsiWithoutProductCode(t *testing.T) {
	fake := installFake(t)
	// MSI-serviced entry whose key name is not a product code: skipped.
	seedUninstallApp(t, fake, "HKLM", "BrokenMsiApp", map[string]regkey.Value{
		"DisplayName":     regStr("Broken MSI App"),
		"UninstallString": regStr("msiexec.exe /x something"),
	})
	err := UninstallADTApplication(context.Background(), UninstallADTApplicationOptions{
		Name: []string{"Broken MSI App"},
	})
	require.NoError(t, err)
}

func TestStartADTMsiProcessValidation(t *testing.T) {
	installFake(t)
	ctx := context.Background()

	_, err := StartADTMsiProcess(ctx, StartADTMsiProcessOptions{})
	require.ErrorIs(t, err, ErrInvalidOption, "Path is required")

	_, err = StartADTMsiProcess(ctx, StartADTMsiProcessOptions{
		Action: "Install",
		Path:   "{26923B43-4D38-484F-9B9E-DE460746276C}",
	})
	require.ErrorIs(t, err, ErrInvalidOption, "product code cannot install")

	_, err = StartADTMsiProcess(ctx, StartADTMsiProcessOptions{
		Action: "Bogus",
		Path:   "app.msi",
	})
	require.ErrorIs(t, err, ErrInvalidOption)

	_, err = StartADTMsiProcess(ctx, StartADTMsiProcessOptions{
		Action: "Install",
		Path:   filepath.Join(t.TempDir(), "missing.msi"),
	})
	require.ErrorIs(t, err, ErrNotFound)
}

func TestStartADTMsiProcessUninstallNotInstalledShortCircuits(t *testing.T) {
	installFake(t) // empty registry: the product is not installed
	res, err := StartADTMsiProcess(context.Background(), StartADTMsiProcessOptions{
		Action: "Uninstall",
		Path:   "{26923B43-4D38-484F-9B9E-DE460746276C}",
	})
	require.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 1605, res.ExitCode)
}

func TestStartADTMspProcessValidation(t *testing.T) {
	installFake(t)
	_, err := StartADTMspProcess(context.Background(), StartADTMspProcessOptions{Path: "patch.msi"})
	require.ErrorIs(t, err, ErrInvalidOption)
}

func TestResolveMsiProduct(t *testing.T) {
	code, err := resolveMsiProduct(nil, "26923b43-4d38-484f-9b9e-de460746276c")
	require.NoError(t, err)
	assert.Equal(t, "{26923B43-4D38-484F-9B9E-DE460746276C}", code)

	dir := t.TempDir()
	msi := filepath.Join(dir, "app.msi")
	require.NoError(t, os.WriteFile(msi, []byte("stub"), 0o644))
	got, err := resolveMsiProduct(nil, msi)
	require.NoError(t, err)
	assert.Equal(t, msi, got)

	_, err = resolveMsiProduct(nil, filepath.Join(dir, "missing.msi"))
	require.ErrorIs(t, err, ErrNotFound)
}

func TestResolveMsiTransforms(t *testing.T) {
	dir := t.TempDir()
	msi := filepath.Join(dir, "app.msi")
	mst := filepath.Join(dir, "custom.mst")
	require.NoError(t, os.WriteFile(msi, []byte("stub"), 0o644))
	require.NoError(t, os.WriteFile(mst, []byte("stub"), 0o644))

	resolved := resolveMsiTransforms(msi, []string{`.\custom.mst`, "absent.mst"})
	assert.Equal(t, []string{mst, "absent.mst"}, resolved)

	// Product codes leave transforms untouched.
	resolved = resolveMsiTransforms("{26923B43-4D38-484F-9B9E-DE460746276C}", []string{"a.mst"})
	assert.Equal(t, []string{"a.mst"}, resolved)
}

func TestApplicationLabel(t *testing.T) {
	assert.Equal(t, "App 1.0", applicationLabel(InstalledApplication{DisplayName: "App", DisplayVersion: "1.0"}))
	assert.Equal(t, "App 1.0", applicationLabel(InstalledApplication{DisplayName: "App 1.0", DisplayVersion: "1.0"}))
	assert.Equal(t, "App", applicationLabel(InstalledApplication{DisplayName: "App"}))
}
