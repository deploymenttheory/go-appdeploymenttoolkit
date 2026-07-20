package deploy

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/logging"
)

// LogSeverity classifies a log entry.
type LogSeverity = logging.Severity

// LogSeverity values.
const (
	LogSeveritySuccess = logging.SeveritySuccess
	LogSeverityInfo    = logging.SeverityInfo
	LogSeverityWarning = logging.SeverityWarning
	LogSeverityError   = logging.SeverityError
)

// LogEntry is one rendered toolkit log entry, as delivered to the
// Hooks.OnLogEntry callbacks.
type LogEntry = logging.Entry

// LogEntryOptions mirrors the parameters of Write-ADTLogEntry.
type LogEntryOptions struct {
	Message  []string
	Severity LogSeverity
	// Source defaults to the calling function's name.
	Source string
	// ScriptSection defaults to the session's current InstallPhase.
	ScriptSection string
}

// WriteLogEntry writes one or more messages to the active session's log
// file (CMTrace or Legacy per config) and console echo.
//
// NOTE: Source defaulting resolves the caller two frames up; platform SDKs
// re-exporting this function must use a var alias (`var WriteADTLogEntry =
// deploy.WriteLogEntry`), never a wrapper func, or the extra frame corrupts
// the attribution.
func WriteLogEntry(ctx context.Context, opts LogEntryOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("adt: %w", err)
	}
	s, err := Current()
	if err != nil {
		return err
	}
	source := opts.Source
	if source == "" {
		source = callerName()
	}
	for _, msg := range opts.Message {
		s.WriteLog(msg, opts.Severity, source, opts.ScriptSection)
	}
	return nil
}

// callerName resolves the toolkit consumer's function name for log Source
// defaulting, mirroring PSADT's use of the caller's command name.
func callerName() string {
	pc, _, _, ok := runtime.Caller(2)
	if !ok {
		return "PSAppDeployToolkit"
	}
	fn := runtime.FuncForPC(pc)
	if fn == nil {
		return "PSAppDeployToolkit"
	}
	name := fn.Name()
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	return name
}
