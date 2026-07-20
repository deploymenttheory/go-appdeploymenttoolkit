package deferral

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
)

func newStore(t *testing.T) (*Store, *regkey.Fake) {
	t.Helper()
	fake := regkey.NewFake()
	s, err := NewStore(fake, `HKLM:\SOFTWARE`, "Contoso_App_1.0_x64_EN_01")
	require.NoError(t, err)
	return s, fake
}

func TestRoundTrip(t *testing.T) {
	s, fake := newStore(t)

	times := uint32(3)
	deadline := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	interval := 4*time.Hour + 30*time.Minute
	require.NoError(t, s.Set(History{TimesRemaining: &times, Deadline: &deadline, RunInterval: &interval}))

	// Stored where PSADT stores it, in PSADT's formats.
	v, err := fake.GetValue("HKLM", `SOFTWARE\PSAppDeployToolkit\DeferHistory\Contoso_App_1.0_x64_EN_01`, "DeferDeadline")
	require.NoError(t, err)
	assert.Equal(t, "2026-08-01T12:00:00.0000000Z", v.Data)
	v, err = fake.GetValue("HKLM", `SOFTWARE\PSAppDeployToolkit\DeferHistory\Contoso_App_1.0_x64_EN_01`, "DeferRunInterval")
	require.NoError(t, err)
	assert.Equal(t, "04:30:00", v.Data)

	got, err := s.Get()
	require.NoError(t, err)
	require.NotNil(t, got.TimesRemaining)
	assert.Equal(t, uint32(3), *got.TimesRemaining)
	require.NotNil(t, got.Deadline)
	assert.True(t, got.Deadline.Equal(deadline))
	require.NotNil(t, got.RunInterval)
	assert.Equal(t, interval, *got.RunInterval)
}

func TestGetAbsentReturnsEmpty(t *testing.T) {
	s, _ := newStore(t)
	h, err := s.Get()
	require.NoError(t, err)
	assert.Nil(t, h.TimesRemaining)
	assert.Nil(t, h.Deadline)
}

func TestReset(t *testing.T) {
	s, _ := newStore(t)
	times := uint32(1)
	require.NoError(t, s.Set(History{TimesRemaining: &times}))
	require.NoError(t, s.Reset())
	h, err := s.Get()
	require.NoError(t, err)
	assert.Nil(t, h.TimesRemaining)
	// Resetting twice is fine.
	require.NoError(t, s.Reset())
}

func TestTimeSpanFormats(t *testing.T) {
	cases := map[time.Duration]string{
		90 * time.Minute:                  "01:30:00",
		26*time.Hour + 15*time.Minute:     "1.02:15:00",
		-(2*time.Hour + 5*time.Second):    "-02:00:05",
		1500 * time.Millisecond:           "00:00:01.5000000",
	}
	for d, want := range cases {
		assert.Equal(t, want, formatTimeSpan(d), "formatting %v", d)
		parsed, err := parseTimeSpan(want)
		require.NoError(t, err, "parsing %s", want)
		assert.Equal(t, d, parsed, "round-trip %s", want)
	}
}

func TestParseDotNetRoundTripTimestamps(t *testing.T) {
	// .NET "O" format with 7 fractional digits and offset.
	got, err := parseRoundTrip("2026-08-01T12:00:00.0000000+02:00")
	require.NoError(t, err)
	assert.Equal(t, 10, got.UTC().Hour())
}
