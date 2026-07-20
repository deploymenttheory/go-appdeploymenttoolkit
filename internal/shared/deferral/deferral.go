// Package deferral persists PSAppDeployToolkit deferral history in the
// registry, byte-compatible with PSADT's DeploymentSession defer storage so
// Go deployments honor defer counts written by PowerShell deployments and
// vice versa: values live at
// <RegPath>\PSAppDeployToolkit\DeferHistory\<InstallName> with
// DeferTimesRemaining (DWORD), DeferDeadline (UTC ISO-8601 string),
// DeferRunInterval (.NET TimeSpan "c" string) and DeferRunIntervalLastTime
// (UTC ISO-8601 string).
package deferral

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/regkey"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// History is the parsed deferral state for one install name.
type History struct {
	TimesRemaining      *uint32        // nil when never set
	Deadline            *time.Time     // nil when no deadline
	RunInterval         *time.Duration // minimum interval between prompts
	RunIntervalLastTime *time.Time
}

// Store reads and writes History under a session-specific registry key.
type Store struct {
	backend regkey.Backend
	hive    string
	path    string // e.g. `SOFTWARE\PSAppDeployToolkit\DeferHistory\<InstallName>`
}

// NewStore builds a Store rooted at regPath (config Toolkit.RegPath, e.g.
// "HKLM:\SOFTWARE") for the given install name.
func NewStore(backend regkey.Backend, regPath, installName string) (*Store, error) {
	hive, sub, err := regkey.SplitRoot(regPath)
	if err != nil {
		return nil, err
	}
	path := `PSAppDeployToolkit\DeferHistory\` + installName
	if sub != "" {
		path = sub + `\` + path
	}
	return &Store{backend: backend, hive: hive, path: path}, nil
}

// Get returns the stored history; a zero History (all nil) when absent.
func (s *Store) Get() (History, error) {
	h := History{}
	vals, err := s.backend.EnumValues(s.hive, s.path)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return h, nil
		}
		return h, err
	}
	if v, ok := vals["DeferTimesRemaining"]; ok {
		if n, ok := v.Data.(uint32); ok {
			h.TimesRemaining = &n
		}
	}
	if v, ok := vals["DeferDeadline"]; ok {
		if t, err := parseRoundTrip(v.Data); err == nil {
			h.Deadline = &t
		}
	}
	if v, ok := vals["DeferRunInterval"]; ok {
		if d, err := parseTimeSpan(v.Data); err == nil {
			h.RunInterval = &d
		}
	}
	if v, ok := vals["DeferRunIntervalLastTime"]; ok {
		if t, err := parseRoundTrip(v.Data); err == nil {
			h.RunIntervalLastTime = &t
		}
	}
	return h, nil
}

// Set writes the non-nil fields of h, mirroring SetDeferHistory in
// DeploymentSession.cs (times as DWORD, timestamps as UTC round-trip strings,
// interval as invariant TimeSpan).
func (s *Store) Set(h History) error {
	if h.TimesRemaining != nil {
		v := regkey.Value{Kind: regkey.KindDWord, Data: *h.TimesRemaining}
		if err := s.backend.SetValue(s.hive, s.path, "DeferTimesRemaining", v); err != nil {
			return err
		}
	}
	if h.Deadline != nil {
		v := regkey.Value{Kind: regkey.KindString, Data: formatRoundTrip(*h.Deadline)}
		if err := s.backend.SetValue(s.hive, s.path, "DeferDeadline", v); err != nil {
			return err
		}
	}
	if h.RunInterval != nil {
		v := regkey.Value{Kind: regkey.KindString, Data: formatTimeSpan(*h.RunInterval)}
		if err := s.backend.SetValue(s.hive, s.path, "DeferRunInterval", v); err != nil {
			return err
		}
	}
	if h.RunIntervalLastTime != nil {
		v := regkey.Value{Kind: regkey.KindString, Data: formatRoundTrip(*h.RunIntervalLastTime)}
		if err := s.backend.SetValue(s.hive, s.path, "DeferRunIntervalLastTime", v); err != nil {
			return err
		}
	}
	return nil
}

// Reset removes the stored history entirely.
func (s *Store) Reset() error {
	err := s.backend.DeleteKey(s.hive, s.path, true)
	if errors.Is(err, errs.ErrNotFound) {
		return nil
	}
	return err
}

// formatRoundTrip renders a UTC .NET round-trip ("O") timestamp.
func formatRoundTrip(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.0000000Z")
}

func parseRoundTrip(data any) (time.Time, error) {
	s, ok := data.(string)
	if !ok {
		return time.Time{}, errs.Wrap("deferral: timestamp type", errs.ErrInvalidOption)
	}
	for _, layout := range []string{"2006-01-02T15:04:05.0000000Z07:00", "2006-01-02T15:04:05.0000000Z", time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, errs.Wrap("deferral: parsing timestamp "+s, errs.ErrInvalidOption)
}

// formatTimeSpan renders a .NET TimeSpan constant ("c") string:
// [-][d.]hh:mm:ss[.fffffff].
func formatTimeSpan(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign = "-"
		d = -d
	}
	days := d / (24 * time.Hour)
	d -= days * 24 * time.Hour
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute
	d -= m * time.Minute
	sec := d / time.Second
	d -= sec * time.Second
	out := fmt.Sprintf("%s%02d:%02d:%02d", sign, h, m, sec)
	if days > 0 {
		out = fmt.Sprintf("%s%d.%02d:%02d:%02d", sign, days, h, m, sec)
	}
	if d > 0 {
		out += fmt.Sprintf(".%07d", d/100) // ticks (100ns)
	}
	return out
}

func parseTimeSpan(data any) (time.Duration, error) {
	s, ok := data.(string)
	if !ok {
		return 0, errs.Wrap("deferral: timespan type", errs.ErrInvalidOption)
	}
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	var days int64
	if dpart, rest, ok := strings.Cut(
		s,
		".",
	); ok && strings.Contains(rest, ":") &&
		!strings.Contains(dpart, ":") {
		if _, err := fmt.Sscanf(dpart, "%d", &days); err != nil {
			return 0, errs.Wrap("deferral: parsing timespan days "+s, errs.ErrInvalidOption)
		}
		s = rest
	}
	var h, m int64
	var sec float64
	if _, err := fmt.Sscanf(s, "%d:%d:%f", &h, &m, &sec); err != nil {
		return 0, errs.Wrap("deferral: parsing timespan "+s, errs.ErrInvalidOption)
	}
	d := time.Duration(days)*24*time.Hour + time.Duration(h)*time.Hour +
		time.Duration(m)*time.Minute + time.Duration(sec*float64(time.Second))
	if neg {
		d = -d
	}
	return d, nil
}
