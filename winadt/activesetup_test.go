package winadt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActiveSetupKeyPath(t *testing.T) {
	assert.Equal(t,
		`HKLM\SOFTWARE\Microsoft\Active Setup\Installed Components\MyApp`,
		activeSetupKeyPath("HKLM", "MyApp", false))
	assert.Equal(t,
		`HKLM\SOFTWARE\Wow6432Node\Microsoft\Active Setup\Installed Components\MyApp`,
		activeSetupKeyPath("HKLM", "MyApp", true))
	assert.Equal(t,
		`HKCU\Software\Microsoft\Active Setup\Installed Components\MyApp`,
		activeSetupKeyPath("HKCU", "MyApp", false))
	assert.Equal(t,
		`HKCU\Software\Wow6432Node\Microsoft\Active Setup\Installed Components\MyApp`,
		activeSetupKeyPath("HKCU", "MyApp", true))
}

func TestActiveSetupPerUserSubkey(t *testing.T) {
	assert.Equal(t,
		`Software\Microsoft\Active Setup\Installed Components\MyApp`,
		activeSetupPerUserSubkey(`HKCU\Software\Microsoft\Active Setup\Installed Components\MyApp`))
}

func TestActiveSetupRegistryValues(t *testing.T) {
	t.Run("hklm enabled", func(t *testing.T) {
		got := activeSetupRegistryValues("Desc", "25.07,18", `"C:\a.exe"`, "en", true, false)
		require.Len(t, got, 5)
		assert.Equal(t, activeSetupRegistryEntry{"(Default)", "Desc", RegistryValueKindString}, got[0])
		assert.Equal(t, activeSetupRegistryEntry{"Version", "25,07,18", RegistryValueKindString}, got[1])
		assert.Equal(t, activeSetupRegistryEntry{"StubPath", `"C:\a.exe"`, RegistryValueKindExpandString}, got[2])
		assert.Equal(t, activeSetupRegistryEntry{"Locale", "en", RegistryValueKindString}, got[3])
		assert.Equal(t, activeSetupRegistryEntry{"IsInstalled", uint32(1), RegistryValueKindDWord}, got[4])
	})

	t.Run("hklm disabled sets IsInstalled 0", func(t *testing.T) {
		got := activeSetupRegistryValues("Desc", "1", "sp", "", true, true)
		require.Len(t, got, 4) // no Locale
		assert.Equal(t, activeSetupRegistryEntry{"IsInstalled", uint32(0), RegistryValueKindDWord}, got[3])
	})

	t.Run("hkcu omits IsInstalled", func(t *testing.T) {
		got := activeSetupRegistryValues("Desc", "1", "sp", "", false, false)
		require.Len(t, got, 3)
		for _, e := range got {
			assert.NotEqual(t, "IsInstalled", e.name)
		}
	})
}

func TestBuildActiveSetupStub(t *testing.T) {
	t.Run("exe no args", func(t *testing.T) {
		stub, err := buildActiveSetupStub(`C:\app.exe`, nil, "")
		require.NoError(t, err)
		assert.Equal(t, `C:\app.exe`, stub.CUStubExePath)
		assert.Equal(t, "", stub.CUArguments)
		assert.Equal(t, `"C:\app.exe"`, stub.StubPath)
	})

	t.Run("exe with single arg", func(t *testing.T) {
		stub, err := buildActiveSetupStub(`C:\app.exe`, []string{"/Silent"}, "")
		require.NoError(t, err)
		assert.Equal(t, "/Silent", stub.CUArguments)
		assert.Equal(t, `"C:\app.exe" /Silent`, stub.StubPath)
	})

	t.Run("vbs uses wscript", func(t *testing.T) {
		stub, err := buildActiveSetupStub(`C:\s.vbs`, nil, "")
		require.NoError(t, err)
		assert.Contains(t, stub.CUStubExePath, "wscript.exe")
		assert.Equal(t, `//nologo "C:\s.vbs"`, stub.CUArguments)
	})

	t.Run("cmd escapes metacharacters without space", func(t *testing.T) {
		stub, err := buildActiveSetupStub(`C:\a(b)&c.cmd`, nil, "")
		require.NoError(t, err)
		assert.Contains(t, stub.CUStubExePath, "cmd.exe")
		assert.Equal(t, `/C "C:\a^(b^)^&c.cmd"`, stub.CUArguments)
	})

	t.Run("ps1 with execution policy", func(t *testing.T) {
		stub, err := buildActiveSetupStub(`C:\s.ps1`, nil, "Bypass")
		require.NoError(t, err)
		assert.Contains(t, stub.CUStubExePath, "powershell.exe")
		assert.Contains(t, stub.CUArguments, "-ExecutionPolicy Bypass")
		assert.Contains(t, stub.CUArguments, `-NoProfile -NoLogo -WindowStyle Hidden -File "C:\s.ps1"`)
	})

	t.Run("unsupported extension", func(t *testing.T) {
		_, err := buildActiveSetupStub(`C:\s.txt`, nil, "")
		assert.ErrorIs(t, err, ErrInvalidOption)
	})
}

func TestJoinActiveSetupArguments(t *testing.T) {
	assert.Equal(t, "", joinActiveSetupArguments(nil))
	assert.Equal(t, "/S %path%", joinActiveSetupArguments([]string{"/S %path%"}))
	assert.Equal(t, `/S "C:\a b"`, joinActiveSetupArguments([]string{"/S", `C:\a b`}))
}

func TestFormatActiveSetupVersion(t *testing.T) {
	got := formatActiveSetupVersion(time.Date(2025, 7, 18, 9, 5, 3, 0, time.UTC))
	assert.Equal(t, "2507,1809,0503", got)
}
