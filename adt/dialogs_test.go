package adt

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func u32(v uint32) *uint32 { return &v }

func TestComputeDeferralStateDisabled(t *testing.T) {
	st := computeDeferralState(false, 3, nil, time.Time{}, time.Now())
	assert.False(t, st.Allowed)
	assert.Zero(t, st.Remaining)
}

func TestComputeDeferralStateWithTimes(t *testing.T) {
	st := computeDeferralState(true, 3, nil, time.Time{}, time.Now())
	assert.True(t, st.Allowed)
	assert.Equal(t, 3, st.Remaining)
	assert.False(t, st.Expired)
}

func TestComputeDeferralStateHistoryWins(t *testing.T) {
	// History (1 remaining) overrides the requested DeferTimes (5).
	st := computeDeferralState(true, 5, u32(1), time.Time{}, time.Now())
	assert.True(t, st.Allowed)
	assert.Equal(t, 1, st.Remaining)
}

func TestComputeDeferralStateExhausted(t *testing.T) {
	st := computeDeferralState(true, 5, u32(0), time.Time{}, time.Now())
	assert.False(t, st.Allowed)
	assert.True(t, st.Expired)
	assert.Zero(t, st.Remaining)
}

func TestComputeDeferralStateDeadlineFuture(t *testing.T) {
	future := time.Now().Add(48 * time.Hour)
	st := computeDeferralState(true, 0, nil, future, time.Now())
	assert.True(t, st.Allowed)
	assert.True(t, st.HasDeadline)
	assert.WithinDuration(t, future, st.Deadline, time.Second)
}

func TestComputeDeferralStateDeadlinePast(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	st := computeDeferralState(true, 0, nil, past, time.Now())
	assert.False(t, st.Allowed)
	assert.True(t, st.Expired)
}

func TestFluentAccentColor(t *testing.T) {
	assert.Equal(t, "", fluentAccentColor(0))
	assert.Equal(t, "#0078D4", fluentAccentColor(0x0078D4))
	// High byte (alpha) is ignored.
	assert.Equal(t, "#0078D4", fluentAccentColor(0xFF0078D4))
}

func TestWelcomeCountdown(t *testing.T) {
	// Force countdown always wins.
	got := welcomeCountdown(
		ShowADTInstallationWelcomeOptions{ForceCloseProcessesCountdown: 30, CloseProcessesCountdown: 99},
		deferralState{Allowed: true},
	)
	assert.Equal(t, 30, got)

	// Normal countdown applies only when deferral is not allowed.
	assert.Equal(t, 0, welcomeCountdown(
		ShowADTInstallationWelcomeOptions{CloseProcessesCountdown: 45},
		deferralState{Allowed: true},
	))
	assert.Equal(t, 45, welcomeCountdown(
		ShowADTInstallationWelcomeOptions{CloseProcessesCountdown: 45},
		deferralState{Allowed: false},
	))
}

func TestButtonText(t *testing.T) {
	opts := ShowADTInstallationPromptOptions{
		ButtonLeftText:   "Yes",
		ButtonMiddleText: "Maybe",
		ButtonRightText:  "No",
	}
	assert.Equal(t, "Yes", buttonText(opts, "Left"))
	assert.Equal(t, "Maybe", buttonText(opts, "Middle"))
	assert.Equal(t, "No", buttonText(opts, "Right"))
	assert.Equal(t, "Timeout", buttonText(opts, "Timeout"))
}
