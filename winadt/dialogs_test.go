package winadt

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/deferral"
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

func TestWelcomeCountdownForceCountdown(t *testing.T) {
	// ForceCountdown applies even while deferral is on offer.
	assert.Equal(t, 60, welcomeCountdown(
		ShowADTInstallationWelcomeOptions{ForceCountdown: 60},
		deferralState{Allowed: true},
	))
	// ForceCloseProcessesCountdown still wins over ForceCountdown.
	assert.Equal(t, 30, welcomeCountdown(
		ShowADTInstallationWelcomeOptions{ForceCloseProcessesCountdown: 30, ForceCountdown: 60},
		deferralState{Allowed: true},
	))
	assert.True(t, forcedWelcomeCountdown(
		ShowADTInstallationWelcomeOptions{ForceCountdown: 60}, deferralState{Allowed: true}))
	assert.False(t, forcedWelcomeCountdown(
		ShowADTInstallationWelcomeOptions{CloseProcessesCountdown: 45}, deferralState{}))
}

func TestDeferRunIntervalDue(t *testing.T) {
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	past := now.Add(-2 * time.Hour)
	recent := now.Add(-10 * time.Minute)

	assert.True(t, deferRunIntervalDue(nil, time.Hour, now), "no history means due")
	assert.True(t, deferRunIntervalDue(&past, 0, now), "no interval means due")
	assert.True(t, deferRunIntervalDue(&past, time.Hour, now), "elapsed interval is due")
	assert.False(t, deferRunIntervalDue(&recent, time.Hour, now), "inside the interval is not due")
}

func TestDeferRunIntervalEarlyDefer(t *testing.T) {
	opts := testSessionOptions(t)
	deferred := 0
	opts.Hooks.OnDefer = append(opts.Hooks.OnDefer, func(ctx context.Context, s *DeploymentSession) {
		deferred++
	})
	opts.DeployMode = DeployModeInteractive
	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	defer CloseADTSession(context.Background(), s)

	// Seed history: last prompt just happened.
	now := time.Now()
	interval := 4 * time.Hour
	require.NoError(t, s.SetDeferHistory(deferral.History{
		RunInterval:         &interval,
		RunIntervalLastTime: &now,
	}))

	res, err := ShowADTInstallationWelcome(context.Background(), ShowADTInstallationWelcomeOptions{
		CloseProcesses:   []ProcessObject{{Name: "definitely-not-running-proc"}},
		AllowDefer:       true,
		DeferTimes:       3,
		DeferRunInterval: interval,
	})
	require.ErrorIs(t, err, ErrDeferred)
	assert.True(t, res.Deferred)
	assert.Equal(t, 1, deferred, "OnDefer hook fires on the run-interval early defer")

	// The deferral count was not consumed.
	h, err := s.DeferHistory()
	require.NoError(t, err)
	assert.Nil(t, h.TimesRemaining)
}

func TestBuildCloseAppsPayloadNewOptions(t *testing.T) {
	opts := testSessionOptions(t)
	opts.DeployMode = DeployModeSilent
	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	defer CloseADTSession(context.Background(), s)

	p := buildCloseAppsPayload(s, ShowADTInstallationWelcomeOptions{
		ForceCountdown:           120,
		CustomMessageText:        "custom banner text",
		NotTopMost:               true,
		AllowDeferCloseProcesses: true,
	}, nil, deferralState{Allowed: true, Remaining: 2})

	require.NotNil(t, p.CloseApps)
	assert.Equal(t, 120, p.CloseApps.CountdownSeconds)
	assert.True(t, p.CloseApps.ForcedCountdown)
	assert.True(t, p.CloseApps.ContinueOnProcessClosure)
	assert.Equal(t, "custom banner text", p.CloseApps.CustomMessage)
	assert.True(t, p.Base.NotTopMost)
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
