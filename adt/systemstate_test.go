package adt

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

func TestPendingRebootStatus_NoSignals(t *testing.T) {
	backend := regkey.NewFake()
	info, err := pendingRebootStatus(backend)
	require.NoError(t, err)
	assert.False(t, info.IsSystemRebootPending)
	assert.False(t, info.IsCBServicingRebootPending)
	assert.False(t, info.IsWindowsUpdateRebootPending)
	assert.False(t, info.IsFileRenameRebootPending)
	assert.Empty(t, info.Reasons)
	// SCCM is always reported as unevaluated (no WMI).
	assert.NotEmpty(t, info.ErrorMsg)
}

func TestPendingRebootStatus_SystemSignals(t *testing.T) {
	backend := regkey.NewFake()
	require.NoError(t, backend.CreateKey("HKLM", rebootKeyCBServicing))
	require.NoError(t, backend.CreateKey("HKLM", rebootKeyWindowsUpd))

	info, err := pendingRebootStatus(backend)
	require.NoError(t, err)
	assert.True(t, info.IsCBServicingRebootPending)
	assert.True(t, info.IsWindowsUpdateRebootPending)
	assert.True(t, info.IsSystemRebootPending)
	assert.Contains(t, info.Reasons, "Component Based Servicing")
	assert.Contains(t, info.Reasons, "Windows Update")
}

func TestPendingRebootStatus_FileRenames(t *testing.T) {
	backend := regkey.NewFake()
	require.NoError(t, backend.SetValue("HKLM", rebootKeySessionMgr, rebootValFileRenames, regkey.Value{
		Kind: regkey.KindMultiString,
		Data: []string{`\??\C:\foo.tmp`, "", `C:\foo.dll`},
	}))

	info, err := pendingRebootStatus(backend)
	require.NoError(t, err)
	assert.True(t, info.IsFileRenameRebootPending)
	assert.True(t, info.IsSystemRebootPending)
	assert.Equal(t, []string{`\??\C:\foo.tmp`, `C:\foo.dll`}, info.PendingFileRenameOperations)
	assert.Contains(t, info.Reasons, "Pending File Rename Operations")
}

func TestPendingRebootStatus_AppVAndIntuneExcludedFromSystem(t *testing.T) {
	backend := regkey.NewFake()
	require.NoError(t, backend.CreateKey("HKLM", rebootKeyAppV))
	require.NoError(t, backend.CreateKey("HKLM", rebootKeyIntune))

	info, err := pendingRebootStatus(backend)
	require.NoError(t, err)
	assert.True(t, info.IsAppVRebootPending)
	assert.True(t, info.IsIntuneClientRebootPending)
	// App-V and Intune do not, on their own, mark a system reboot pending.
	assert.False(t, info.IsSystemRebootPending)
	assert.Contains(t, info.Reasons, "App-V Pending Tasks")
	assert.Contains(t, info.Reasons, "Intune Management Extension")
}

func TestGetADTPendingReboot_Facade(t *testing.T) {
	fake := installFake(t)
	require.NoError(t, fake.CreateKey("HKLM", rebootKeyCBServicing))

	info, err := GetADTPendingReboot(context.Background())
	require.NoError(t, err)
	assert.True(t, info.IsSystemRebootPending)
	assert.NotEmpty(t, info.ComputerName)
}

func TestGetADTPendingReboot_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := GetADTPendingReboot(ctx)
	require.Error(t, err)
}

func TestMicrophoneInUse(t *testing.T) {
	t.Run("in use when LastUsedTimeStop is zero", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.SetValue("HKCU", microphoneConsentKey+`\App.Teams`, "LastUsedTimeStop", regkey.Value{Kind: regkey.KindQWord, Data: uint64(0)}))
		inUse, err := microphoneInUse(backend)
		require.NoError(t, err)
		assert.True(t, inUse)
	})

	t.Run("not in use when LastUsedTimeStop is non-zero", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.SetValue("HKCU", microphoneConsentKey+`\App.Teams`, "LastUsedTimeStop", regkey.Value{Kind: regkey.KindQWord, Data: uint64(133000000000000000)}))
		inUse, err := microphoneInUse(backend)
		require.NoError(t, err)
		assert.False(t, inUse)
	})

	t.Run("detects nested NonPackaged apps", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.SetValue("HKCU", microphoneConsentKey+`\NonPackaged\C#Users#me#app.exe`, "LastUsedTimeStop", regkey.Value{Kind: regkey.KindQWord, Data: uint64(0)}))
		inUse, err := microphoneInUse(backend)
		require.NoError(t, err)
		assert.True(t, inUse)
	})

	t.Run("false when store is absent", func(t *testing.T) {
		backend := regkey.NewFake()
		inUse, err := microphoneInUse(backend)
		require.NoError(t, err)
		assert.False(t, inUse)
	})
}

func TestEspRegistryActive(t *testing.T) {
	base := enrollmentsKey + `\ABC-123\FirstSync\S-1-5-21-1-2-3-1001`

	t.Run("active when IsSyncDone is zero", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.SetValue("HKLM", base, "IsSyncDone", regkey.Value{Kind: regkey.KindDWord, Data: uint32(0)}))
		active, err := espRegistryActive(backend)
		require.NoError(t, err)
		assert.True(t, active)
	})

	t.Run("active when IsSyncDone is missing", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.CreateKey("HKLM", base))
		active, err := espRegistryActive(backend)
		require.NoError(t, err)
		assert.True(t, active)
	})

	t.Run("inactive when IsSyncDone is set", func(t *testing.T) {
		backend := regkey.NewFake()
		require.NoError(t, backend.SetValue("HKLM", base, "IsSyncDone", regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)}))
		active, err := espRegistryActive(backend)
		require.NoError(t, err)
		assert.False(t, active)
	})

	t.Run("inactive with no enrollments", func(t *testing.T) {
		backend := regkey.NewFake()
		active, err := espRegistryActive(backend)
		require.NoError(t, err)
		assert.False(t, active)
	})
}

func TestPresentationEnabledUsers(t *testing.T) {
	enabled := `S-1-5-21-1-2-3-1001`
	disabled := `S-1-5-21-1-2-3-1002`
	// hkuBackend (defined in registry_test.go) emulates enumerating the
	// HKEY_USERS root, which the bare Fake cannot represent.
	backend := &hkuBackend{
		Fake:  regkey.NewFake(),
		roots: []string{"S-1-5-18", enabled, disabled, enabled + "_Classes"},
	}
	require.NoError(t, backend.SetValue("HKU", enabled+`\`+presentationActivityKey, "Activity", regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)}))
	require.NoError(t, backend.SetValue("HKU", disabled+`\`+presentationActivityKey, "Activity", regkey.Value{Kind: regkey.KindDWord, Data: uint32(0)}))
	// A system SID must be ignored even if it somehow carries the value.
	require.NoError(t, backend.SetValue("HKU", `S-1-5-18\`+presentationActivityKey, "Activity", regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)}))

	sids, err := presentationEnabledUsers(backend)
	require.NoError(t, err)
	assert.Equal(t, []string{enabled}, sids)
}

func TestAnyNetworkConnection(t *testing.T) {
	// Result depends on the host, but the probe must not error.
	_, err := anyNetworkConnection()
	require.NoError(t, err)
}

func TestToastNotificationModeString(t *testing.T) {
	assert.Equal(t, "Unrestricted", ToastNotificationModeUnrestricted.String())
	assert.Equal(t, "PriorityOnly", ToastNotificationModePriorityOnly.String())
	assert.Equal(t, "AlarmsOnly", ToastNotificationModeAlarmsOnly.String())
	assert.Equal(t, "ToastNotificationMode(9)", ToastNotificationMode(9).String())
}

func TestTestADTMicrophoneInUse_Facade(t *testing.T) {
	fake := installFake(t)
	require.NoError(t, fake.SetValue("HKCU", microphoneConsentKey+`\App.Zoom`, "LastUsedTimeStop", regkey.Value{Kind: regkey.KindQWord, Data: uint64(0)}))
	inUse, err := TestADTMicrophoneInUse(context.Background())
	require.NoError(t, err)
	assert.True(t, inUse)
}
