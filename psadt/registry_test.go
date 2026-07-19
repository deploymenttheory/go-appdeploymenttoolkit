package psadt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// installFake injects a fresh in-memory registry backend for the duration of
// the test.
func installFake(t *testing.T) *regkey.Fake {
	t.Helper()
	fake := regkey.NewFake()
	registryBackendHook = func() regkey.Backend { return fake }
	t.Cleanup(func() { registryBackendHook = nil })
	return fake
}

func TestConvertADTRegistryPath(t *testing.T) {
	cases := []struct {
		name string
		key  string
		wow  bool
		want string
	}{
		{"drive form", `HKLM:\SOFTWARE\App`, false, `HKLM:\SOFTWARE\App`},
		{"drive form no backslash", `HKLM:SOFTWARE\App`, false, `HKLM:\SOFTWARE\App`},
		{"bare backslash form", `HKLM\SOFTWARE\App`, false, `HKLM:\SOFTWARE\App`},
		{"full hive form", `HKEY_LOCAL_MACHINE\SOFTWARE\App`, false, `HKLM:\SOFTWARE\App`},
		{"provider prefixed", `Microsoft.PowerShell.Core\Registry::HKEY_CURRENT_USER\Environment`, false, `HKCU:\Environment`},
		{"registry prefixed", `Registry::HKEY_USERS\S-1-5-21-1\Environment`, false, `HKU:\S-1-5-21-1\Environment`},
		{"bare hive", `HKLM`, false, `HKLM:`},
		{"trailing backslash trimmed", `HKLM:\SOFTWARE\`, false, `HKLM:\SOFTWARE`},
		{"classes root", `HKCR\Directory\shell`, false, `HKCR:\Directory\shell`},
		{"current config", `HKCC\System`, false, `HKCC:\System`},
		{"wow software subkey", `HKLM:\SOFTWARE\App`, true, `HKLM:\SOFTWARE\Wow6432Node\App`},
		{"wow software root", `HKLM:\SOFTWARE`, true, `HKLM:\SOFTWARE\Wow6432Node`},
		{"wow software case-insensitive", `HKLM\Software\App`, true, `HKLM:\SOFTWARE\Wow6432Node\App`},
		{"wow classes clsid", `HKCR\CLSID\{00000000-0000-0000-0000-000000000000}`, true, `HKCR:\Wow6432Node\CLSID\{00000000-0000-0000-0000-000000000000}`},
		{"wow hklm classes clsid", `HKLM\SOFTWARE\Classes\CLSID\{1}`, true, `HKLM:\SOFTWARE\Classes\Wow6432Node\CLSID\{1}`},
		{
			"wow active setup",
			`HKCU\Software\Microsoft\Active Setup\Installed Components\{1}`,
			true,
			`HKCU:\Software\Wow6432Node\Microsoft\Active Setup\Installed Components\{1}`,
		},
		{"wow does not touch hkcu software", `HKCU:\Software\App`, true, `HKCU:\Software\App`},
		{"wow does not touch system", `HKLM:\SYSTEM\CurrentControlSet`, true, `HKLM:\SYSTEM\CurrentControlSet`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ConvertADTRegistryPath(tc.key, tc.wow)
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestConvertADTRegistryPathErrors(t *testing.T) {
	_, err := ConvertADTRegistryPath("", false)
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	_, err = ConvertADTRegistryPath(`FOO:\Bar`, false)
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	_, err = ConvertADTRegistryPath(`C:\Windows`, false)
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}

func TestRegistryKeyRoundTrip(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\Test`

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "Str", Value: "hello"}))
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "DWord", Value: 7}))
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "QWord", Value: int64(1) << 40}))
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "Multi", Value: []string{"a", "b"}}))
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "Bin", Value: []byte{1, 2, 3}}))

	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "Str"})
	require.NoError(t, err)
	assert.Equal(t, "hello", v)

	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "DWord"})
	require.NoError(t, err)
	assert.Equal(t, uint32(7), v)

	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "QWord"})
	require.NoError(t, err)
	assert.Equal(t, uint64(1)<<40, v)

	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "Multi"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b"}, v)

	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "Bin"})
	require.NoError(t, err)
	assert.Equal(t, []byte{1, 2, 3}, v)

	// Whole-key read returns every value.
	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key})
	require.NoError(t, err)
	all, ok := v.(map[string]any)
	require.True(t, ok)
	assert.Len(t, all, 5)
	assert.Equal(t, "hello", all["Str"])
}

func TestRegistryKeyHiveFormsInterchangeable(t *testing.T) {
	installFake(t)
	ctx := t.Context()

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
		Key:   `HKEY_LOCAL_MACHINE\SOFTWARE\PSADT\Forms`,
		Name:  "V",
		Value: "x",
	}))

	// The same key addressed via the drive form resolves identically.
	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: `HKLM:\SOFTWARE\PSADT\Forms`, Name: "V"})
	require.NoError(t, err)
	assert.Equal(t, "x", v)
}

func TestRegistryKeyWow6432Node(t *testing.T) {
	fake := installFake(t)
	ctx := t.Context()

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
		Key:         `HKLM:\SOFTWARE\PSADT\Wow`,
		Name:        "V",
		Value:       "32bit",
		Wow6432Node: true,
	}))

	// The value landed under SOFTWARE\Wow6432Node.
	raw, err := fake.GetValue("HKLM", `SOFTWARE\Wow6432Node\PSADT\Wow`, "V")
	require.NoError(t, err)
	assert.Equal(t, "32bit", raw.Data)

	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: `HKLM:\SOFTWARE\PSADT\Wow`, Name: "V", Wow6432Node: true})
	require.NoError(t, err)
	assert.Equal(t, "32bit", v)
}

func TestRegistryKeyDefaultValue(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\Default`

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "(Default)", Value: "def"}))

	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "(Default)"})
	require.NoError(t, err)
	assert.Equal(t, "def", v)

	exists, err := TestADTRegistryValue(ctx, TestADTRegistryValueOptions{Key: key, Name: "(Default)"})
	require.NoError(t, err)
	assert.True(t, exists)
}

func TestRegistryKeyExpandString(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\Expand`
	t.Setenv("PSADT_TEST_VAR", "expanded")

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{
		Key:   key,
		Name:  "E",
		Value: `%PSADT_TEST_VAR%\bin;%PSADT_UNDEFINED%`,
		Type:  RegistryValueKindExpandString,
	}))

	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "E"})
	require.NoError(t, err)
	assert.Equal(t, `expanded\bin;%PSADT_UNDEFINED%`, v)

	v, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, Name: "E", DoNotExpandEnvironmentNames: true})
	require.NoError(t, err)
	assert.Equal(t, `%PSADT_TEST_VAR%\bin;%PSADT_UNDEFINED%`, v)
}

func TestGetADTRegistryKeyMissing(t *testing.T) {
	installFake(t)
	ctx := t.Context()

	_, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: `HKLM:\SOFTWARE\PSADT\Absent`})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	// Key exists but the value does not.
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: `HKLM:\SOFTWARE\PSADT\Empty`}))
	_, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: `HKLM:\SOFTWARE\PSADT\Empty`, Name: "Nope"})
	assert.ErrorIs(t, err, winerr.ErrNotFound)
}

func TestGetADTRegistryKeyReturnEmptyKeyIfExists(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\EmptyKey`

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key}))

	_, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	v, err := GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key, ReturnEmptyKeyIfExists: true})
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, v)
}

func TestTestADTRegistryValue(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\TestVal`

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "Present", Value: "y"}))

	exists, err := TestADTRegistryValue(ctx, TestADTRegistryValueOptions{Key: key, Name: "Present"})
	require.NoError(t, err)
	assert.True(t, exists)

	exists, err = TestADTRegistryValue(ctx, TestADTRegistryValueOptions{Key: key, Name: "Absent"})
	require.NoError(t, err)
	assert.False(t, exists)

	// A missing key reports false, not an error.
	exists, err = TestADTRegistryValue(ctx, TestADTRegistryValueOptions{Key: `HKLM:\SOFTWARE\PSADT\NoKey`, Name: "X"})
	require.NoError(t, err)
	assert.False(t, exists)
}

func TestRemoveADTRegistryKey(t *testing.T) {
	installFake(t)
	ctx := t.Context()
	const key = `HKLM:\SOFTWARE\PSADT\Remove`

	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key, Name: "A", Value: "1"}))
	require.NoError(t, SetADTRegistryKey(ctx, SetADTRegistryKeyOptions{Key: key + `\Child`, Name: "B", Value: "2"}))

	// Deleting a named value.
	require.NoError(t, RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key, Name: "A"}))
	exists, err := TestADTRegistryValue(ctx, TestADTRegistryValueOptions{Key: key, Name: "A"})
	require.NoError(t, err)
	assert.False(t, exists)

	// Deleting a key with subkeys requires Recurse.
	err = RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key})
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)

	require.NoError(t, RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key, Recurse: true}))
	_, err = GetADTRegistryKey(ctx, GetADTRegistryKeyOptions{Key: key})
	assert.ErrorIs(t, err, winerr.ErrNotFound)

	// Deleting a key that does not exist succeeds (PSADT logs and returns).
	require.NoError(t, RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key}))
	require.NoError(t, RemoveADTRegistryKey(ctx, RemoveADTRegistryKeyOptions{Key: key, Name: "A"}))
}

// hkuBackend wraps the Fake to emulate enumerating the HKEY_USERS root,
// which the Fake cannot represent with an empty subkey path.
type hkuBackend struct {
	*regkey.Fake
	roots []string
}

func (b *hkuBackend) EnumSubkeys(hive, path string) ([]string, error) {
	if hive == "HKU" && path == "" {
		return b.roots, nil
	}
	return b.Fake.EnumSubkeys(hive, path)
}

func TestInvokeADTAllUsersRegistryAction(t *testing.T) {
	backend := &hkuBackend{
		Fake: regkey.NewFake(),
		roots: []string{
			".DEFAULT",
			"S-1-5-18",
			"S-1-5-19",
			"S-1-5-21-1111111111-222222222-333333333-1001",
			"S-1-5-21-1111111111-222222222-333333333-1001_Classes",
			"S-1-5-21-1111111111-222222222-333333333-1002",
		},
	}
	registryBackendHook = func() regkey.Backend { return backend }
	t.Cleanup(func() { registryBackendHook = nil })

	var visited []string
	err := InvokeADTAllUsersRegistryAction(t.Context(), func(_ context.Context, userSID string) error {
		visited = append(visited, userSID)
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"S-1-5-21-1111111111-222222222-333333333-1001",
		"S-1-5-21-1111111111-222222222-333333333-1002",
	}, visited)
}

func TestInvokeADTAllUsersRegistryActionCollectsErrors(t *testing.T) {
	backend := &hkuBackend{
		Fake:  regkey.NewFake(),
		roots: []string{"S-1-5-21-1-1-1-1001", "S-1-5-21-1-1-1-1002"},
	}
	registryBackendHook = func() regkey.Backend { return backend }
	t.Cleanup(func() { registryBackendHook = nil })

	var visited []string
	err := InvokeADTAllUsersRegistryAction(t.Context(), func(_ context.Context, userSID string) error {
		visited = append(visited, userSID)
		if userSID == "S-1-5-21-1-1-1-1001" {
			return winerr.Wrap("test failure", winerr.ErrNotFound)
		}
		return nil
	})
	// The failing user is reported but the remaining users are still visited.
	assert.ErrorIs(t, err, winerr.ErrNotFound)
	assert.Len(t, visited, 2)

	// A nil action is rejected.
	err = InvokeADTAllUsersRegistryAction(t.Context(), nil)
	assert.ErrorIs(t, err, winerr.ErrInvalidOption)
}
