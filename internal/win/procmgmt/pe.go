package procmgmt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// errNotPE marks files without a valid MZ/PE header pair.
var errNotPE = errors.New("procmgmt: not a PE file")

// IMAGE_FILE_MACHINE values recognized by Get-ADTPEFileArchitecture.
const (
	machineI386  = 0x014C
	machineIA64  = 0x0200
	machineARMNT = 0x01C4
	machineAMD64 = 0x8664
	machineARM64 = 0xAA64
)

// dos header offsets: e_lfanew lives at byte 60, and the COFF machine field
// sits 4 bytes past the "PE\0\0" signature — the same constants PSADT uses
// ($PE_POINTER_OFFSET = 60, $MACHINE_OFFSET = 4).
const (
	pePointerOffset = 60
	machineOffset   = 4
)

// PEFileArchitecture ports Get-ADTPEFileArchitecture: it reads the PE COFF
// header machine field and returns the PSADT-style architecture label
// ("x86", "x64", "ARM64", "ARM", "IA64"). The header is parsed directly
// (debug/pe rejects machine values outside its own support matrix, which
// would hide the unknown-machine case PSADT reports).
func PEFileArchitecture(path string) (string, error) {
	f, err := os.Open(
		path,
	) //#nosec G304 -- inspecting caller-specified files is this function's purpose
	if err != nil {
		return "", fmt.Errorf("procmgmt: opening PE file %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	machine, err := peMachine(f)
	if err != nil {
		return "", fmt.Errorf("procmgmt: reading PE header of %q: %w", path, err)
	}
	switch machine {
	case machineI386:
		return "x86", nil
	case machineAMD64:
		return "x64", nil
	case machineARM64:
		return "ARM64", nil
	case machineARMNT:
		return "ARM", nil
	case machineIA64:
		return "IA64", nil
	default:
		return "", fmt.Errorf(
			"procmgmt: PE file %q has unrecognized machine 0x%04X: %w",
			path, machine, winerr.ErrNotFound,
		)
	}
}

// peMachine validates the MZ and PE signatures and returns the COFF machine
// field.
func peMachine(r io.ReaderAt) (uint16, error) {
	var dos [pePointerOffset + 4]byte
	if _, err := r.ReadAt(dos[:], 0); err != nil {
		return 0, fmt.Errorf("reading DOS header: %w", errNotPE)
	}
	if dos[0] != 'M' || dos[1] != 'Z' {
		return 0, fmt.Errorf("missing MZ signature: %w", errNotPE)
	}
	peOffset := int64(binary.LittleEndian.Uint32(dos[pePointerOffset:]))

	var header [machineOffset + 2]byte // "PE\0\0" + machine uint16
	if _, err := r.ReadAt(header[:], peOffset); err != nil {
		return 0, fmt.Errorf("reading COFF header: %w", errNotPE)
	}
	if header[0] != 'P' || header[1] != 'E' || header[2] != 0 || header[3] != 0 {
		return 0, fmt.Errorf("missing PE signature: %w", errNotPE)
	}
	return binary.LittleEndian.Uint16(header[machineOffset:]), nil
}
