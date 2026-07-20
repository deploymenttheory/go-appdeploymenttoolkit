package dialogclient

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

func baseOpts() ipc.BaseDialogOptions {
	return ipc.BaseDialogOptions{
		Title:       "Contoso App 1.0",
		Subtitle:    "Contoso - App Installation",
		AccentColor: "#0078d4",
		FluentStyle: true,
	}
}

func TestBuildViewModelCloseAppsWithDeferAndCountdown(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCloseApps,
		Base:       baseOpts(),
		CloseApps: &ipc.CloseAppsOptions{
			Message:            "Please save your work.",
			Apps:               []ipc.AppToClose{{Name: "excel", Description: "Microsoft Excel"}},
			AllowDefer:         true,
			DeferralsRemaining: 3,
			CountdownSeconds:   120,
			ButtonContinueText: "Close Apps & Install",
			ButtonDeferText:    "Defer",
			ButtonCloseText:    "Close Programs",
		},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)

	assert.Equal(t, string(ipc.DialogCloseApps), vm.Type)
	assert.True(t, vm.Fluent)
	assert.Equal(t, "#0078d4", vm.AccentColor)
	assert.Len(t, vm.Apps, 1)
	assert.Equal(t, "Microsoft Excel", vm.Apps[0].Description)

	// Buttons: Close (apps running), Defer (allowed), Continue (primary).
	require.Len(t, vm.Buttons, 3)
	assert.Equal(t, ButtonClose, vm.Buttons[0].ID)
	assert.Equal(t, ButtonDefer, vm.Buttons[1].ID)
	assert.Equal(t, ButtonContinue, vm.Buttons[2].ID)
	assert.Equal(t, "primary", vm.Buttons[2].Kind)

	assert.True(t, vm.ShowDeferral)
	assert.Equal(t, 3, vm.DeferralsRemaining)
	assert.Equal(t, 120, vm.CountdownSeconds)
	assert.Equal(t, ButtonContinue, vm.CountdownButton)
}

func TestBuildViewModelCloseAppsNoDeferNoApps(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCloseApps,
		Base:       baseOpts(),
		CloseApps: &ipc.CloseAppsOptions{
			Message:            "Select Install to continue.",
			ButtonContinueText: "Install",
		},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)
	// No Close button (no apps), no Defer (not allowed): only Continue.
	require.Len(t, vm.Buttons, 1)
	assert.Equal(t, ButtonContinue, vm.Buttons[0].ID)
	assert.False(t, vm.ShowDeferral)
	assert.Zero(t, vm.CountdownSeconds)
}

func TestBuildViewModelCustomButtons(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCustom,
		Base:       baseOpts(),
		Custom: &ipc.CustomOptions{
			Message:         "Proceed?",
			Icon:            "Question",
			ButtonLeftText:  "Yes",
			ButtonRightText: "No",
		},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)
	require.Len(t, vm.Buttons, 2)
	assert.Equal(t, ButtonLeft, vm.Buttons[0].ID)
	assert.Equal(t, "secondary", vm.Buttons[0].Kind)
	assert.Equal(t, ButtonRight, vm.Buttons[1].ID)
	assert.Equal(t, "primary", vm.Buttons[1].Kind) // last present button is primary
	assert.Equal(t, "Question", vm.Icon)
}

func TestBuildViewModelInput(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogInput,
		Base:       baseOpts(),
		Input: &ipc.InputOptions{
			Message:         "Enter name:",
			DefaultValue:    "admin",
			ButtonLeftText:  "Cancel",
			ButtonRightText: "OK",
		},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)
	require.NotNil(t, vm.Input)
	assert.Equal(t, "admin", vm.Input.DefaultValue)
	require.Len(t, vm.Buttons, 2)
}

func TestBuildViewModelListRequiresItems(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogListSelection,
		Base:       baseOpts(),
		List: &ipc.ListOptions{
			Message:         "Pick:",
			ButtonRightText: "OK",
		},
	}
	_, err := BuildViewModel(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, winerr.ErrInvalidOption))
}

func TestBuildViewModelRestartCountdown(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogRestart,
		Base:       baseOpts(),
		Restart: &ipc.RestartOptions{
			Message:              "Restart required.",
			MessageRestart:       "Auto-restarting at countdown end.",
			CountdownSeconds:     60,
			ButtonRestartNowText: "Restart Now",
			ButtonRestartLater:   "Minimize",
		},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)
	require.Len(t, vm.Buttons, 2)
	assert.Equal(t, ButtonRestartLater, vm.Buttons[0].ID)
	assert.Equal(t, ButtonRestartNow, vm.Buttons[1].ID)
	assert.Equal(t, "primary", vm.Buttons[1].Kind)
	assert.Equal(t, 60, vm.CountdownSeconds)
	assert.Equal(t, ButtonRestartNow, vm.CountdownButton)
	assert.Equal(t, "Auto-restarting at countdown end.", vm.Labels["messageRestart"])
}

func TestBuildViewModelDialogBoxRejected(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogBox,
		Base:       baseOpts(),
		Box:        &ipc.DialogBoxOptions{Text: "hi", Buttons: "OK"},
	}
	_, err := BuildViewModel(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, winerr.ErrInvalidOption))
}

func TestBuildViewModelMissingTitle(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCustom,
		Custom:     &ipc.CustomOptions{Message: "x", ButtonRightText: "OK"},
	}
	_, err := BuildViewModel(p)
	require.Error(t, err)
	assert.True(t, errors.Is(err, winerr.ErrInvalidOption))
}

func TestViewModelJSONRoundTrip(t *testing.T) {
	p := ipc.ModalDialogPayload{
		DialogType: ipc.DialogCustom,
		Base:       baseOpts(),
		Custom:     &ipc.CustomOptions{Message: "hi", ButtonRightText: "OK"},
	}
	vm, err := BuildViewModel(p)
	require.NoError(t, err)
	js, err := vm.JSON()
	require.NoError(t, err)

	var back ViewModel
	require.NoError(t, json.Unmarshal([]byte(js), &back))
	assert.Equal(t, vm.Title, back.Title)
	assert.Equal(t, vm.Buttons, back.Buttons)
}

func TestFormatCountdown(t *testing.T) {
	assert.Equal(t, "00:00:00", FormatCountdown(0))
	assert.Equal(t, "00:00:00", FormatCountdown(-5))
	assert.Equal(t, "00:02:05", FormatCountdown(125))
	assert.Equal(t, "01:01:01", FormatCountdown(3661))
}
