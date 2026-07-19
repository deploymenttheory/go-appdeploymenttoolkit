package adt

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildRegSvr32Args(t *testing.T) {
	cases := []struct {
		name   string
		action string
		asUser bool
		path   string
		want   string
	}{
		{"register", "Register", false, `C:\a b\x.dll`, `/s "C:\a b\x.dll"`},
		{"unregister", "Unregister", false, `C:\x.dll`, `/s /u "C:\x.dll"`},
		{"register as user", "Register", true, `C:\x.dll`, `/s /n /i:user "C:\x.dll"`},
		{"unregister as user", "Unregister", true, `C:\x.dll`, `/s /u /n /i:user "C:\x.dll"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, buildRegSvr32Args(tc.action, tc.asUser, tc.path))
		})
	}
}

func TestCanonicalRegSvr32Action(t *testing.T) {
	got, err := canonicalRegSvr32Action("register")
	require.NoError(t, err)
	assert.Equal(t, "Register", got)

	got, err = canonicalRegSvr32Action("UNREGISTER")
	require.NoError(t, err)
	assert.Equal(t, "Unregister", got)

	_, err = canonicalRegSvr32Action("bogus")
	assert.ErrorIs(t, err, ErrInvalidOption)
}

func TestResolveRegSvr32Path(t *testing.T) {
	const windir = `C:\Windows`
	cases := []struct {
		name             string
		arch             string
		osIs64, procIs64 bool
		want             string
		wantErr          bool
	}{
		{"x64 dll on x64 os, x64 proc", "x64", true, true, `C:\Windows\System32\regsvr32.exe`, false},
		{"x64 dll on x64 os, x86 proc", "x64", true, false, `C:\Windows\sysnative\regsvr32.exe`, false},
		{"x86 dll on x64 os", "x86", true, true, `C:\Windows\SysWOW64\regsvr32.exe`, false},
		{"x86 dll on x86 os", "x86", false, false, `C:\Windows\System32\regsvr32.exe`, false},
		{"x64 dll on x86 os", "x64", false, false, "", true},
		{"unsupported arch", "ARM64", true, true, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveRegSvr32Path(tc.arch, tc.osIs64, tc.procIs64, windir)
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrInvalidOption)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestResolveRegSvr32PathNoWindir(t *testing.T) {
	_, err := resolveRegSvr32Path("x64", true, true, "")
	assert.Error(t, err)
}
