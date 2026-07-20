package procmgmt

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RunningProcesses ports the enumeration behind Get-ADTRunningProcesses: it
// snapshots the process table (Toolhelp32) and returns every process whose
// image name matches one of the specs. Description resolution mirrors
// PSADT's RunningProcessInfo: the spec's override wins, then the image's
// version-resource FileDescription, then the bare process name.
func RunningProcesses(specs []ProcessSpec) ([]RunningProcess, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	bySpecName := make(map[string]ProcessSpec, len(specs))
	for _, spec := range specs {
		bySpecName[normalizeProcessName(spec.Name)] = spec
	}

	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("procmgmt: CreateToolhelp32Snapshot: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snapshot) }()

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var out []RunningProcess
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		exeName := windows.UTF16ToString(entry.ExeFile[:])
		spec, ok := bySpecName[normalizeProcessName(exeName)]
		if !ok {
			continue
		}
		var sessionID uint32
		_ = windows.ProcessIdToSessionId(entry.ProcessID, &sessionID) // best effort; 0 on failure
		name := trimExeSuffix(exeName)
		description := spec.Description
		if description == "" {
			description = imageFileDescription(entry.ProcessID)
		}
		if description == "" {
			description = name
		}
		out = append(out, RunningProcess{
			Name:        name,
			Description: description,
			PID:         entry.ProcessID,
			SessionID:   sessionID,
		})
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("procmgmt: walking process snapshot: %w", err)
	}
	return out, nil
}

// imageFileDescription resolves a process image's version-resource
// FileDescription. Every failure path returns "" — the caller falls back to
// the process name, matching PSADT.
func imageFileDescription(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer func() { _ = windows.CloseHandle(h) }()

	const pathBufLen = uint32(windows.MAX_LONG_PATH)
	buf := make([]uint16, pathBufLen)
	size := pathBufLen
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return versionFileDescription(windows.UTF16ToString(buf[:size]))
}

// versionFileDescription reads FileDescription from a file's version
// resource using its first declared translation.
func versionFileDescription(imagePath string) string {
	size, err := windows.GetFileVersionInfoSize(imagePath, nil)
	if err != nil || size == 0 {
		return ""
	}
	block := make([]byte, size)
	if err := windows.GetFileVersionInfo(
		imagePath,
		0,
		size,
		unsafe.Pointer(&block[0]),
	); err != nil {
		return ""
	}

	// First language/codepage pair from \VarFileInfo\Translation.
	var langPtr unsafe.Pointer
	var langLen uint32
	if err := windows.VerQueryValue(
		unsafe.Pointer(&block[0]),
		`\VarFileInfo\Translation`,
		unsafe.Pointer(&langPtr),
		&langLen,
	); err != nil || langLen < 4 {
		return ""
	}
	type langCodepage struct{ Lang, Codepage uint16 }
	lc := *(*langCodepage)(langPtr)

	var descPtr unsafe.Pointer
	var descLen uint32
	subBlock := fmt.Sprintf(`\StringFileInfo\%04x%04x\FileDescription`, lc.Lang, lc.Codepage)
	if err := windows.VerQueryValue(
		unsafe.Pointer(&block[0]),
		subBlock,
		unsafe.Pointer(&descPtr),
		&descLen,
	); err != nil || descLen == 0 {
		return ""
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(descPtr), descLen))
}
