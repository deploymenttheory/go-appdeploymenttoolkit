package procmgmt

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// jobObjectBasicAccountingInformation mirrors
// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION (information class 1), which x/sys
// does not define.
type jobObjectBasicAccountingInformation struct {
	TotalUserTime             int64
	TotalKernelTime           int64
	ThisPeriodTotalUserTime   int64
	ThisPeriodTotalKernelTime int64
	TotalPageFaultCount       uint32
	TotalProcesses            uint32
	ActiveProcesses           uint32
	TotalTerminatedProcesses  uint32
}

// jobObjectBasicAccountingInformationClass is the information class for the
// struct above.
const jobObjectBasicAccountingInformationClass = 1

// jobEmptyPollSlice is the interval at which waitEmpty re-samples the job's
// active-process count.
const jobEmptyPollSlice = 250 * time.Millisecond

// jobHandle wraps a Windows job object used to track a launched process tree
// (Start-ADTProcess -WaitForChildProcesses / -KillChildProcessesWithParent).
type jobHandle struct {
	h           windows.Handle
	killOnClose bool
}

// newJob creates an anonymous job object; when killOnClose is set, closing
// the handle terminates every process still in the job.
func newJob(killOnClose bool) (*jobHandle, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("procmgmt: CreateJobObject: %w", err)
	}
	if killOnClose {
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
			BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
				LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
			},
		}
		if _, err := windows.SetInformationJobObject(
			h,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(h)
			return nil, fmt.Errorf("procmgmt: SetInformationJobObject: %w", err)
		}
	}
	return &jobHandle{h: h, killOnClose: killOnClose}, nil
}

// assign places the process (and its future children) into the job.
func (j *jobHandle) assign(pid int) error {
	//#nosec G115 -- pid is a Windows process ID
	proc, err := windows.OpenProcess(
		windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE,
		false,
		uint32(pid),
	)
	if err != nil {
		return fmt.Errorf("procmgmt: OpenProcess(%d) for job assignment: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(proc) }()
	if err := windows.AssignProcessToJobObject(j.h, proc); err != nil {
		return fmt.Errorf("procmgmt: AssignProcessToJobObject: %w", err)
	}
	return nil
}

// waitEmpty polls until the job has no active processes (the root process
// and all its children have exited) or ctx is done.
func (j *jobHandle) waitEmpty(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("procmgmt: waiting for child processes: %w", err)
		}
		var info jobObjectBasicAccountingInformation
		if err := windows.QueryInformationJobObject(
			j.h,
			jobObjectBasicAccountingInformationClass,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
			nil,
		); err != nil {
			return fmt.Errorf("procmgmt: QueryInformationJobObject: %w", err)
		}
		if info.ActiveProcesses == 0 {
			return nil
		}
		select {
		case <-ctx.Done():
		case <-time.After(jobEmptyPollSlice):
		}
	}
}

// Close releases the job handle; with killOnClose set this terminates any
// process still in the job.
func (j *jobHandle) Close() error {
	if j.h == 0 {
		return nil
	}
	err := windows.CloseHandle(j.h)
	j.h = 0
	return err
}

// resumeProcessMainThreads resumes every suspended thread of a process
// created with CREATE_SUSPENDED (a freshly created process has exactly one).
func resumeProcessMainThreads(pid int) error {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPTHREAD, 0)
	if err != nil {
		return fmt.Errorf("procmgmt: thread snapshot: %w", err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var entry windows.ThreadEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	resumed := false
	for err := windows.Thread32First(snap, &entry); err == nil; err = windows.Thread32Next(snap, &entry) {
		//#nosec G115 -- pid is a Windows process ID
		if entry.OwnerProcessID != uint32(pid) {
			continue
		}
		h, err := windows.OpenThread(windows.THREAD_SUSPEND_RESUME, false, entry.ThreadID)
		if err != nil {
			continue
		}
		if _, err := windows.ResumeThread(h); err == nil {
			resumed = true
		}
		_ = windows.CloseHandle(h)
	}
	if !resumed {
		return fmt.Errorf("procmgmt: no thread of process %d could be resumed", pid)
	}
	return nil
}
