package adt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// RegisterADTDll is the Go port of Register-ADTDll: it registers a DLL via
// regsvr32.exe, delegating to InvokeADTRegSvr32 with the Register action.
func RegisterADTDll(ctx context.Context, filePath string) error {
	return InvokeADTRegSvr32(ctx, InvokeADTRegSvr32Options{FilePath: filePath, Action: "Register"})
}

// UnregisterADTDll is the Go port of Unregister-ADTDll: it unregisters a DLL
// via regsvr32.exe, delegating to InvokeADTRegSvr32 with the Unregister action.
func UnregisterADTDll(ctx context.Context, filePath string) error {
	return InvokeADTRegSvr32(ctx, InvokeADTRegSvr32Options{FilePath: filePath, Action: "Unregister"})
}

// InvokeADTRegSvr32Options mirrors the parameters of Invoke-ADTRegSvr32.
type InvokeADTRegSvr32Options struct {
	// FilePath is the .dll file to (un)register.
	FilePath string
	// Action is "Register" or "Unregister".
	Action string
	// AsUser registers the DLL for the current user only (regsvr32 /n /i:user).
	AsUser bool
}

// InvokeADTRegSvr32 is the Go port of Invoke-ADTRegSvr32: it registers or
// unregisters a DLL with regsvr32.exe, selecting the regsvr32 build that
// matches the DLL's architecture and executing it via StartADTProcess.
func InvokeADTRegSvr32(ctx context.Context, opts InvokeADTRegSvr32Options) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: InvokeADTRegSvr32: %w", err)
	}
	action, err := canonicalRegSvr32Action(opts.Action)
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Ext(opts.FilePath), ".dll") {
		return fmt.Errorf("adt: FilePath %q must be a .dll file: %w", opts.FilePath, ErrInvalidOption)
	}
	if info, err := os.Stat(opts.FilePath); err != nil || info.IsDir() {
		return fmt.Errorf("adt: DLL file [%s] does not exist: %w", opts.FilePath, ErrNotFound)
	}
	logToSession(fmt.Sprintf("%s DLL file [%s].", action, opts.FilePath), LogSeverityInfo, "InvokeADTRegSvr32")

	arch, err := GetADTPEFileArchitecture(ctx, opts.FilePath)
	if err != nil {
		return err
	}
	osIs64, procIs64 := systemBitness()
	regsvr32Path, err := resolveRegSvr32Path(arch, osIs64, procIs64, windowsDir())
	if err != nil {
		return err
	}
	args := buildRegSvr32Args(action, opts.AsUser, opts.FilePath)

	launch := StartADTProcess
	if opts.AsUser {
		launch = func(ctx context.Context, o StartADTProcessOptions) (*ProcessResult, error) {
			return StartADTProcessAsUser(ctx, StartADTProcessAsUserOptions{StartADTProcessOptions: o})
		}
	}
	_, err = launch(ctx, StartADTProcessOptions{
		FilePath:         regsvr32Path,
		ArgumentList:     args,
		CreateNoWindow:   true,
		SuccessExitCodes: []int{0},
	})
	return err
}

// canonicalRegSvr32Action validates and title-cases the action verb.
func canonicalRegSvr32Action(action string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(action)) {
	case "register":
		return "Register", nil
	case "unregister":
		return "Unregister", nil
	default:
		return "", fmt.Errorf("adt: Action %q must be Register or Unregister: %w", action, ErrInvalidOption)
	}
}

// buildRegSvr32Args composes the regsvr32.exe argument string for the action,
// mirroring Invoke-ADTRegSvr32's $ActionParameters switch.
func buildRegSvr32Args(action string, asUser bool, filePath string) string {
	var b strings.Builder
	b.WriteString("/s")
	if action == "Unregister" {
		b.WriteString(" /u")
	}
	if asUser {
		b.WriteString(" /n /i:user")
	}
	b.WriteString(` "`)
	b.WriteString(filePath)
	b.WriteByte('"')
	return b.String()
}

// resolveRegSvr32Path ports Invoke-ADTRegSvr32's selection of the regsvr32.exe
// build matching the DLL architecture (arch is "x86" or "x64"): it picks the
// native, SysWOW64 or sysnative copy according to the OS and process bitness.
func resolveRegSvr32Path(arch string, osIs64, procIs64 bool, windir string) (string, error) {
	if windir == "" {
		return "", winerr.Wrap("adt: Windows directory unresolved", winerr.ErrNotFound)
	}
	// Windows system paths are always built with backslashes so the result is
	// deterministic regardless of the host running these path builders.
	regsvr := func(subdir string) string { return windir + `\` + subdir + `\regsvr32.exe` }

	switch strings.ToLower(arch) {
	case "x64", "x86":
	default:
		return "", fmt.Errorf(
			"adt: file has architecture [%s]; only 32-bit or 64-bit DLLs can be registered: %w",
			arch, ErrInvalidOption)
	}

	is64 := strings.EqualFold(arch, "x64")
	switch {
	case osIs64 && is64:
		if procIs64 {
			return regsvr("System32"), nil
		}
		return regsvr("sysnative"), nil
	case osIs64 && !is64:
		return regsvr("SysWOW64"), nil
	case !osIs64 && !is64:
		return regsvr("System32"), nil
	default:
		return "", fmt.Errorf(
			"adt: cannot register a 64-bit DLL on a 32-bit operating system: %w", ErrInvalidOption)
	}
}

// systemBitness reports whether the operating system and current process are
// 64-bit. It derives the process bitness from the build target and the OS
// bitness from the processor-architecture environment variables Windows sets.
func systemBitness() (osIs64, procIs64 bool) {
	procIs64 = runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"
	if os.Getenv("PROCESSOR_ARCHITEW6432") != "" {
		return true, procIs64
	}
	switch strings.ToUpper(os.Getenv("PROCESSOR_ARCHITECTURE")) {
	case "AMD64", "ARM64", "IA64":
		return true, procIs64
	case "X86":
		return false, procIs64
	default:
		return procIs64, procIs64
	}
}

// windowsDir resolves the Windows directory (%WINDIR% / %SystemRoot%).
func windowsDir() string {
	if windir := os.Getenv("WINDIR"); windir != "" {
		return windir
	}
	return os.Getenv("SystemRoot")
}
