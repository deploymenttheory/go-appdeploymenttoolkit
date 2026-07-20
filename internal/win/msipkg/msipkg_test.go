package msipkg

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

func TestExitCodeMessage(t *testing.T) {
	cases := []struct {
		code int
		want string
	}{
		{0, "The action completed successfully. (ERROR_SUCCESS)"},
		{1602, "The user cancelled installation. (ERROR_INSTALL_USEREXIT)"},
		{1603, "A fatal error occurred during installation. (ERROR_INSTALL_FAILURE)"},
		{1605, "This action is only valid for products that are currently installed. (ERROR_UNKNOWN_PRODUCT)"},
		{1618, "Another installation is already in progress. Complete that installation before proceeding with this install. (ERROR_INSTALL_ALREADY_RUNNING)"},
		{3010, "A restart is required to complete the install. This message is indicative of a success. This does not include installs where the ForceReboot action is run. (ERROR_SUCCESS_REBOOT_REQUIRED)"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, ExitCodeMessage(tc.code), "code %d", tc.code)
	}
	for _, code := range []int{1619, 1620, 1622, 1623, 1625, 1636, 1638, 1639, 1640, 1641, 1707} {
		assert.NotContains(t, ExitCodeMessage(code), "Unknown MSI exit code", "code %d", code)
	}
	assert.Equal(t, "Unknown MSI exit code (42).", ExitCodeMessage(42))
}

func TestExitCodeSets(t *testing.T) {
	assert.Equal(t, []int{1641, 3010}, RebootExitCodes())
	assert.Equal(t, []int{0, 1707, 3010, 1641}, SuccessExitCodes())
	for _, code := range RebootExitCodes() {
		assert.True(t, IsRebootExitCode(code))
		assert.True(t, IsSuccessExitCode(code))
	}
	for _, code := range SuccessExitCodes() {
		assert.True(t, IsSuccessExitCode(code))
	}
	assert.False(t, IsRebootExitCode(0))
	assert.False(t, IsSuccessExitCode(1603))
}

func TestNormalizeProductCode(t *testing.T) {
	code, err := NormalizeProductCode("26923b43-4d38-484f-9b9e-de460746276c")
	require.NoError(t, err)
	assert.Equal(t, "{26923B43-4D38-484F-9B9E-DE460746276C}", code)

	code, err = NormalizeProductCode("{26923B43-4D38-484F-9B9E-DE460746276C}")
	require.NoError(t, err)
	assert.Equal(t, "{26923B43-4D38-484F-9B9E-DE460746276C}", code)

	_, err = NormalizeProductCode("setup.msi")
	require.ErrorIs(t, err, winerr.ErrInvalidOption)

	assert.True(t, IsProductCode("{26923B43-4D38-484F-9B9E-DE460746276C}"))
	assert.False(t, IsProductCode(`C:\Files\setup.msi`))
}

// seedApp writes one uninstall key with the given values into the fake.
func seedApp(t *testing.T, fake *regkey.Fake, hive, root, key string, values map[string]regkey.Value) {
	t.Helper()
	path := root + `\` + key
	require.NoError(t, fake.CreateKey(hive, path))
	for name, value := range values {
		require.NoError(t, fake.SetValue(hive, path, name, value))
	}
}

const (
	hklmUninstall    = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`
	hklmWowUninstall = `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`
)

func str(s string) regkey.Value   { return regkey.Value{Kind: regkey.KindString, Data: s} }
func dword(v uint32) regkey.Value { return regkey.Value{Kind: regkey.KindDWord, Data: v} }

// seededFake builds a registry with a representative application mix.
func seededFake(t *testing.T) *regkey.Fake {
	t.Helper()
	fake := regkey.NewFake()
	seedApp(t, fake, "HKLM", hklmUninstall, "{26923B43-4D38-484F-9B9E-DE460746276C}", map[string]regkey.Value{
		"DisplayName":      str("Adobe Acrobat Reader"),
		"DisplayVersion":   str("23.1.0"),
		"Publisher":        str("Adobe"),
		"UninstallString":  str(`MsiExec.exe /X{26923B43-4D38-484F-9B9E-DE460746276C}`),
		"WindowsInstaller": dword(1),
	})
	seedApp(t, fake, "HKLM", hklmWowUninstall, "{11111111-2222-3333-4444-555555555555}", map[string]regkey.Value{
		"DisplayName":      str("Legacy Widget 1.0"),
		"DisplayVersion":   str("1.0"),
		"WindowsInstaller": dword(1),
	})
	seedApp(t, fake, "HKLM", hklmUninstall, "NotepadPlusPlus", map[string]regkey.Value{
		"DisplayName":          str("Notepad++ (64-bit x64)"),
		"DisplayVersion":       str("8.6"),
		"Publisher":            str("Notepad++ Team"),
		"UninstallString":      str(`"C:\Program Files\Notepad++\uninstall.exe"`),
		"QuietUninstallString": str(`"C:\Program Files\Notepad++\uninstall.exe" /S`),
	})
	seedApp(t, fake, "HKCU", hklmUninstall, "UserTool", map[string]regkey.Value{
		"DisplayName":     str("User Tool"),
		"UninstallString": str(`"C:\Users\u\AppData\Local\UserTool\remove.exe"`),
		"SystemComponent": dword(1),
	})
	seedApp(t, fake, "HKLM", hklmUninstall, "KB5006670", map[string]regkey.Value{
		"DisplayName": str("Security Update for Windows (KB5006670)"),
	})
	// Entry without a DisplayName must be skipped.
	seedApp(t, fake, "HKLM", hklmUninstall, "Nameless", map[string]regkey.Value{
		"UninstallString": str(`C:\nameless\remove.exe`),
	})
	return fake
}

func findNames(apps []InstalledApplication) []string {
	names := make([]string, 0, len(apps))
	for _, app := range apps {
		names = append(names, app.DisplayName)
	}
	return names
}

func TestFindInstalledApplicationsAll(t *testing.T) {
	fake := seededFake(t)
	apps, err := FindInstalledApplications(fake, FindOptions{})
	require.NoError(t, err)
	assert.ElementsMatch(t,
		[]string{"Adobe Acrobat Reader", "Legacy Widget 1.0", "Notepad++ (64-bit x64)", "User Tool"},
		findNames(apps))
}

func TestFindInstalledApplicationsIncludesUpdatesWhenAsked(t *testing.T) {
	fake := seededFake(t)
	apps, err := FindInstalledApplications(fake, FindOptions{IncludeUpdatesAndHotfixes: true})
	require.NoError(t, err)
	assert.Contains(t, findNames(apps), "Security Update for Windows (KB5006670)")
}

func TestFindInstalledApplicationsNameMatching(t *testing.T) {
	fake := seededFake(t)

	apps, err := FindInstalledApplications(fake, FindOptions{Names: []string{"acrobat"}})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "Adobe Acrobat Reader", apps[0].DisplayName)

	apps, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{"adobe acrobat reader"}, NameMatch: "Exact",
	})
	require.NoError(t, err)
	require.Len(t, apps, 1)

	apps, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{"Acrobat"}, NameMatch: "Exact",
	})
	require.NoError(t, err)
	assert.Empty(t, apps)

	apps, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{`Notepad\+\+.*x64.*`}, NameMatch: "Regex",
	})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "Notepad++ (64-bit x64)", apps[0].DisplayName)

	apps, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{"legacy*1.?"}, NameMatch: "Wildcard",
	})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.Equal(t, "Legacy Widget 1.0", apps[0].DisplayName)

	_, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{"("}, NameMatch: "Regex",
	})
	require.Error(t, err)

	_, err = FindInstalledApplications(fake, FindOptions{
		Names: []string{"x"}, NameMatch: "Approximate",
	})
	require.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestFindInstalledApplicationsProductCode(t *testing.T) {
	fake := seededFake(t)
	apps, err := FindInstalledApplications(fake, FindOptions{
		ProductCodes: []string{"26923b43-4d38-484f-9b9e-de460746276c"},
	})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	app := apps[0]
	assert.Equal(t, "Adobe Acrobat Reader", app.DisplayName)
	assert.Equal(t, "{26923B43-4D38-484F-9B9E-DE460746276C}", app.ProductCode)
	assert.True(t, app.WindowsInstaller)
	assert.True(t, app.Is64Bit)
	assert.Equal(t, "Adobe", app.Publisher)
	assert.Equal(t, "23.1.0", app.DisplayVersion)

	_, err = FindInstalledApplications(fake, FindOptions{ProductCodes: []string{"not-a-guid"}})
	require.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestFindInstalledApplicationsApplicationType(t *testing.T) {
	fake := seededFake(t)

	apps, err := FindInstalledApplications(fake, FindOptions{ApplicationType: "MSI"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Adobe Acrobat Reader", "Legacy Widget 1.0"}, findNames(apps))

	apps, err = FindInstalledApplications(fake, FindOptions{ApplicationType: "EXE"})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"Notepad++ (64-bit x64)", "User Tool"}, findNames(apps))
	for _, app := range apps {
		assert.False(t, app.WindowsInstaller)
		assert.Empty(t, app.ProductCode)
	}

	_, err = FindInstalledApplications(fake, FindOptions{ApplicationType: "Script"})
	require.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestFindInstalledApplicationsFieldMapping(t *testing.T) {
	fake := seededFake(t)
	apps, err := FindInstalledApplications(fake, FindOptions{Names: []string{"User Tool"}})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	app := apps[0]
	assert.True(t, app.SystemComponent)
	assert.False(t, app.Is64Bit, "HKCU entries never report 64-bit")
	assert.Equal(t, `"C:\Users\u\AppData\Local\UserTool\remove.exe"`, app.UninstallString)
	assert.Empty(t, app.QuietUninstallString)

	apps, err = FindInstalledApplications(fake, FindOptions{Names: []string{"Legacy Widget"}})
	require.NoError(t, err)
	require.Len(t, apps, 1)
	assert.False(t, apps[0].Is64Bit, "WOW6432Node entries are 32-bit")
	assert.Equal(t, "{11111111-2222-3333-4444-555555555555}", apps[0].ProductCode)
}

func TestFindInstalledApplicationsEmptyRegistry(t *testing.T) {
	apps, err := FindInstalledApplications(regkey.NewFake(), FindOptions{})
	require.NoError(t, err)
	assert.Empty(t, apps)
}

func TestWildcardToRegex(t *testing.T) {
	assert.Equal(t, `(?i)^Adobe.*Reader$`, wildcardToRegex("Adobe*Reader"))
	assert.Equal(t, `(?i)^a\.b.c$`, wildcardToRegex("a.b?c"))
}
