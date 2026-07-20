package session

import (
	"os"
	"runtime"
)

// EnvironmentTable is the Go port of the core of PSADT's environment table:
// the resolved machine/user facts sessions and public functions rely on.
// PSADT exposes ~100 variables; this carries the set the toolkit core needs
// and grows as functions are ported.
type EnvironmentTable struct {
	AppDeployToolkitName       string
	EnvUserName                string
	EnvUserDomain              string
	EnvComputerName            string
	EnvSystemDrive             string
	EnvWinDir                  string
	EnvTemp                    string
	EnvProgramFiles            string
	EnvProgramFilesX86         string
	EnvProgramData             string
	EnvAllUsersProfile         string
	EnvUserProfile             string
	EnvCommonDesktop           string
	EnvCommonStartMenu         string
	EnvCommonStartMenuPrograms string
	EnvCommonStartUp           string
	IsAdmin                    bool
	Is64BitOS                  bool
	Is64BitProcess             bool
	OSArchitecture             string // amd64 | arm64 | 386
	ProcessID                  int
}

// newEnvironmentTable resolves the table from the process environment; the
// Windows build augments it with shell folder and token queries.
func newEnvironmentTable(isAdmin bool) *EnvironmentTable {
	get := os.Getenv
	windir := get("SystemRoot")
	if windir == "" {
		windir = get("windir")
	}
	t := &EnvironmentTable{
		AppDeployToolkitName: "PSAppDeployToolkit",
		EnvUserName:          firstNonEmpty(get("USERNAME"), get("USER")),
		EnvUserDomain:        get("USERDOMAIN"),
		EnvComputerName:      firstNonEmpty(get("COMPUTERNAME"), hostname()),
		EnvSystemDrive:       firstNonEmpty(get("SystemDrive"), "C:"),
		EnvWinDir:            windir,
		EnvTemp:              firstNonEmpty(get("TEMP"), get("TMP"), os.TempDir()),
		EnvProgramFiles:      get("ProgramFiles"),
		EnvProgramFilesX86:   get("ProgramFiles(x86)"),
		EnvProgramData:       get("ProgramData"),
		EnvAllUsersProfile:   get("ALLUSERSPROFILE"),
		EnvUserProfile:       firstNonEmpty(get("USERPROFILE"), get("HOME")),
		IsAdmin:              isAdmin,
		Is64BitProcess:       runtime.GOARCH != "386",
		OSArchitecture:       runtime.GOARCH,
		ProcessID:            os.Getpid(),
	}
	if t.EnvProgramData != "" {
		base := t.EnvProgramData + `\Microsoft\Windows`
		t.EnvCommonDesktop = get("PUBLIC") + `\Desktop`
		t.EnvCommonStartMenu = base + `\Start Menu`
		t.EnvCommonStartMenuPrograms = base + `\Start Menu\Programs`
		t.EnvCommonStartUp = base + `\Start Menu\Programs\StartUp`
	}
	t.Is64BitOS = t.EnvProgramFilesX86 != "" || runtime.GOARCH == "arm64" ||
		runtime.GOARCH == "amd64"
	return t
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// firstNonEmptyWithSource returns the first non-empty value and its source
// label from {value, source} pairs; ("", "default") when all are empty.
func firstNonEmptyWithSource(pairs [][2]string) (value, source string) {
	for _, p := range pairs {
		if p[0] != "" {
			return p[0], p[1]
		}
	}
	return "", "default"
}

func hostname() string {
	h, err := os.Hostname()
	if err != nil {
		return ""
	}
	return h
}
