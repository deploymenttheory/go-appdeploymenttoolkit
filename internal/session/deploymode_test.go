package session

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/regkey"
)

// probeSet builds deployModeProbes with benign defaults, overridable per test.
func probeSet(over func(*deployModeProbes)) deployModeProbes {
	p := deployModeProbes{
		oobeComplete:            func() (bool, error) { return true, nil },
		espUserSetupActive:      func() (bool, error) { return false, nil },
		sessionZero:             func() bool { return false },
		processInteractive:      func() bool { return false },
		activeUserPresent:       func() bool { return true },
		processesToCloseRunning: func() (bool, error) { return false, nil },
	}
	if over != nil {
		over(&p)
	}
	return p
}

func TestResolveAutoDeployMode(t *testing.T) {
	procs := []ProcessObject{{Name: "notepad"}}
	cases := []struct {
		name string
		opts Options
		over func(*deployModeProbes)
		want DeployMode
	}{
		{
			name: "oobe incomplete forces noninteractive",
			over: func(p *deployModeProbes) { p.oobeComplete = func() (bool, error) { return false, nil } },
			want: DeployModeNonInteractive,
		},
		{
			name: "oobe incomplete with NoOobeDetection falls through to silent",
			opts: Options{NoOobeDetection: true},
			over: func(p *deployModeProbes) { p.oobeComplete = func() (bool, error) { return false, nil } },
			want: DeployModeSilent, // no processes specified => silent
		},
		{
			name: "esp user setup forces noninteractive",
			over: func(p *deployModeProbes) { p.espUserSetupActive = func() (bool, error) { return true, nil } },
			want: DeployModeNonInteractive,
		},
		{
			name: "esp with NoOobeDetection ignored",
			opts: Options{NoOobeDetection: true, NoProcessDetection: true},
			over: func(p *deployModeProbes) { p.espUserSetupActive = func() (bool, error) { return true, nil } },
			want: DeployModeInteractive,
		},
		{
			name: "session0 with interactivity detection and non-interactive station",
			opts: Options{ProcessInteractivityDetection: true, AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.sessionZero = func() bool { return true }
				p.processesToCloseRunning = func() (bool, error) { return true, nil }
			},
			want: DeployModeSilent,
		},
		{
			name: "session0 no users and non-interactive station",
			opts: Options{AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.sessionZero = func() bool { return true }
				p.activeUserPresent = func() bool { return false }
				p.processesToCloseRunning = func() (bool, error) { return true, nil }
			},
			want: DeployModeSilent,
		},
		{
			name: "session0 no users but interactive station stays interactive",
			opts: Options{AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.sessionZero = func() bool { return true }
				p.activeUserPresent = func() bool { return false }
				p.processInteractive = func() bool { return true }
				p.processesToCloseRunning = func() (bool, error) { return true, nil }
			},
			want: DeployModeInteractive,
		},
		{
			name: "session0 with NoSessionDetection skips silent",
			opts: Options{NoSessionDetection: true, AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.sessionZero = func() bool { return true }
				p.activeUserPresent = func() bool { return false }
				p.processesToCloseRunning = func() (bool, error) { return true, nil }
			},
			want: DeployModeInteractive,
		},
		{
			name: "processes specified but none running forces silent",
			opts: Options{AppProcessesToClose: procs},
			want: DeployModeSilent,
		},
		{
			name: "processes specified and running stays interactive",
			opts: Options{AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.processesToCloseRunning = func() (bool, error) { return true, nil }
			},
			want: DeployModeInteractive,
		},
		{
			name: "no processes specified forces silent (v4.2 semantics)",
			want: DeployModeSilent,
		},
		{
			name: "no processes with NoProcessDetection stays interactive",
			opts: Options{NoProcessDetection: true},
			want: DeployModeInteractive,
		},
		{
			name: "first change wins: oobe beats process detection",
			over: func(p *deployModeProbes) { p.oobeComplete = func() (bool, error) { return false, nil } },
			want: DeployModeNonInteractive,
		},
		{
			name: "probe errors leave mode unchanged",
			opts: Options{AppProcessesToClose: procs},
			over: func(p *deployModeProbes) {
				p.oobeComplete = func() (bool, error) { return false, errors.New("boom") }
				p.processesToCloseRunning = func() (bool, error) { return false, errors.New("boom") }
			},
			want: DeployModeInteractive,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var logs []string
			got := resolveAutoDeployMode(tc.opts, probeSet(tc.over), func(m string) { logs = append(logs, m) })
			assert.Equal(t, tc.want, got)
			assert.NotEmpty(t, logs, "every resolution should log its decisions")
		})
	}
}

func TestEspFirstSyncPending(t *testing.T) {
	sid := "S-1-5-21-1111"
	otherSID := "S-1-5-21-2222"
	firstSync := espEnrollmentsKey + `\{guid-1}\FirstSync`

	newReg := func() regkey.Backend { return regkey.NewFake() }

	t.Run("no enrollments key", func(t *testing.T) {
		pending, err := espFirstSyncPending(newReg(), sid)
		require.NoError(t, err)
		assert.False(t, pending)
	})

	t.Run("missing IsSyncDone means pending", func(t *testing.T) {
		reg := newReg()
		require.NoError(t, reg.CreateKey("HKLM", firstSync+`\`+sid))
		pending, err := espFirstSyncPending(reg, sid)
		require.NoError(t, err)
		assert.True(t, pending)
	})

	t.Run("IsSyncDone zero means pending", func(t *testing.T) {
		reg := newReg()
		require.NoError(t, reg.SetValue("HKLM", firstSync+`\`+sid, "IsSyncDone", regkey.Value{Kind: regkey.KindDWord, Data: uint32(0)}))
		pending, err := espFirstSyncPending(reg, sid)
		require.NoError(t, err)
		assert.True(t, pending)
	})

	t.Run("IsSyncDone one means done", func(t *testing.T) {
		reg := newReg()
		require.NoError(t, reg.SetValue("HKLM", firstSync+`\`+sid, "IsSyncDone", regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)}))
		pending, err := espFirstSyncPending(reg, sid)
		require.NoError(t, err)
		assert.False(t, pending)
	})

	t.Run("scoped to active user SID", func(t *testing.T) {
		reg := newReg()
		// Another user's sync is pending, but the active user's is done.
		require.NoError(t, reg.CreateKey("HKLM", firstSync+`\`+otherSID))
		require.NoError(t, reg.SetValue("HKLM", firstSync+`\`+sid, "IsSyncDone", regkey.Value{Kind: regkey.KindDWord, Data: uint32(1)}))
		pending, err := espFirstSyncPending(reg, sid)
		require.NoError(t, err)
		assert.False(t, pending)
	})

	t.Run("unscoped matches any pending entry", func(t *testing.T) {
		reg := newReg()
		require.NoError(t, reg.CreateKey("HKLM", firstSync+`\`+otherSID))
		pending, err := espFirstSyncPending(reg, "")
		require.NoError(t, err)
		assert.True(t, pending)
	})
}

func TestUserUICulture(t *testing.T) {
	sid := "S-1-5-21-3333"
	profileKey := sid + `\Control Panel\International\User Profile`

	t.Run("multi-string first entry", func(t *testing.T) {
		reg := regkey.NewFake()
		require.NoError(t, reg.SetValue("HKU", profileKey, "Languages",
			regkey.Value{Kind: regkey.KindMultiString, Data: []string{"en-GB", "cy-GB"}}))
		assert.Equal(t, "en-GB", userUICulture(reg, sid))
	})

	t.Run("plain string accepted", func(t *testing.T) {
		reg := regkey.NewFake()
		require.NoError(t, reg.SetValue("HKU", profileKey, "Languages",
			regkey.Value{Kind: regkey.KindString, Data: "fr-FR"}))
		assert.Equal(t, "fr-FR", userUICulture(reg, sid))
	})

	t.Run("missing value yields empty", func(t *testing.T) {
		assert.Equal(t, "", userUICulture(regkey.NewFake(), sid))
	})

	t.Run("empty sid yields empty", func(t *testing.T) {
		assert.Equal(t, "", userUICulture(regkey.NewFake(), ""))
	})
}
