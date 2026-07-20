package fsops

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// FreeDiskSpaceMB is the engine behind Get-ADTFreeDiskSpace: it returns the
// megabytes available to the caller on the volume containing path.
func FreeDiskSpaceMB(path string) (uint64, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, fmt.Errorf("fsops: encoding path %s: %w", path, err)
	}
	var freeToCaller, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(p, &freeToCaller, &total, &totalFree); err != nil {
		return 0, fmt.Errorf("fsops: GetDiskFreeSpaceEx %s: %w", path, err)
	}
	return freeToCaller / (1024 * 1024), nil
}
