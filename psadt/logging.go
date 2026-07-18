package psadt

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/logging"
)

// LogSeverity mirrors PSADT's LogSeverity enum.
type LogSeverity = logging.Severity

// LogSeverity values.
const (
	LogSeveritySuccess = logging.SeveritySuccess
	LogSeverityInfo    = logging.SeverityInfo
	LogSeverityWarning = logging.SeverityWarning
	LogSeverityError   = logging.SeverityError
)

// LogEntryOptions mirrors the parameters of Write-ADTLogEntry.
type LogEntryOptions struct {
	Message  []string
	Severity LogSeverity
	// Source defaults to the calling function's name.
	Source string
	// ScriptSection defaults to the session's current InstallPhase.
	ScriptSection string
}

// WriteADTLogEntry is the Go port of Write-ADTLogEntry: it writes one or
// more messages to the active session's log file (CMTrace or Legacy per
// config) and console echo.
func WriteADTLogEntry(ctx context.Context, opts LogEntryOptions) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("psadt: %w", err)
	}
	s, err := GetADTSession()
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

// NewADTLogFileName is the Go port of New-ADTLogFileName: it returns the
// active session's default log file name with the given discriminator.
func NewADTLogFileName(discriminator string) (string, error) {
	s, err := GetADTSession()
	if err != nil {
		return "", err
	}
	return s.NewLogFileName(discriminator), nil
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
