package winadt

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnLogEntryHookReceivesEntries(t *testing.T) {
	opts := testSessionOptions(t)
	var messages []string
	opts.Hooks.OnLogEntry = append(opts.Hooks.OnLogEntry, func(e LogEntry) {
		messages = append(messages, e.Message)
	})
	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	s.WriteLog("hook probe", LogSeverityInfo, "Test", "")
	CloseADTSession(context.Background(), s)

	assert.Contains(t, messages, "hook probe")
	assert.NotEmpty(t, messages, "opening entries should also reach the hook")
}

func TestSessionFacadeFunctions(t *testing.T) {
	opts := testSessionOptions(t)
	var hookOrder []string
	opts.Hooks.Starting = append(opts.Hooks.Starting, func(ctx context.Context) error {
		hookOrder = append(hookOrder, "starting")
		return nil
	})
	opts.Hooks.Opening = append(opts.Hooks.Opening, func(ctx context.Context, s *DeploymentSession) error {
		hookOrder = append(hookOrder, "opening")
		return nil
	})
	opts.Hooks.Finishing = append(opts.Hooks.Finishing, func(ctx context.Context) error {
		hookOrder = append(hookOrder, "finishing")
		return nil
	})

	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	assert.True(t, TestADTSessionActive())

	got, err := GetADTSession()
	require.NoError(t, err)
	assert.Same(t, s, got)

	cfg, err := GetADTConfig()
	require.NoError(t, err)
	assert.Equal(t, "PSAppDeployToolkit", cfg.Toolkit.CompanyName)

	tbl, err := GetADTStringTable()
	require.NoError(t, err)
	assert.Equal(t, "Restart Required", tbl.MustGet("RestartPrompt.Title", ""))

	env, err := GetADTEnvironmentTable()
	require.NoError(t, err)
	assert.Equal(t, "PSAppDeployToolkit", env.AppDeployToolkitName)

	require.NoError(t, WriteADTLogEntry(context.Background(), LogEntryOptions{Message: []string{"facade log"}}))
	name, err := NewADTLogFileName("Custom")
	require.NoError(t, err)
	assert.Contains(t, name, "_Custom_")

	CloseADTSession(context.Background(), s)
	assert.False(t, TestADTSessionActive())
	assert.Equal(t, []string{"starting", "opening", "finishing"}, hookOrder)

	_, err = GetADTSession()
	assert.ErrorIs(t, err, ErrNoActiveSession)
}

// TestWriteADTLogEntrySourceAttribution pins the var-alias contract: routing
// through winadt's re-export must not add a stack frame, so the defaulted
// Source is the calling function, not the alias package.
func TestWriteADTLogEntrySourceAttribution(t *testing.T) {
	opts := testSessionOptions(t)
	var sources []string
	opts.Hooks.OnLogEntry = append(opts.Hooks.OnLogEntry, func(e LogEntry) {
		sources = append(sources, e.Source)
	})
	s, err := OpenADTSession(context.Background(), opts)
	require.NoError(t, err)
	require.NoError(t, WriteADTLogEntry(context.Background(), LogEntryOptions{Message: []string{"attribution probe"}}))
	CloseADTSession(context.Background(), s)

	found := false
	for _, src := range sources {
		if strings.HasSuffix(src, "TestWriteADTLogEntrySourceAttribution") {
			found = true
		}
		// A wrapper frame would attribute the entry to the engine itself.
		assert.NotEqual(t, "deploy.WriteLogEntry", src,
			"an alias wrapper frame would misattribute the Source")
	}
	assert.True(t, found, "the defaulted Source must be the calling test function; got %v", sources)
}
