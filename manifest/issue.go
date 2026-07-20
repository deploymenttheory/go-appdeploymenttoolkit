package manifest

import "sort"

// Pos is a 1-based line/column position in the manifest file.
type Pos struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// Severity classifies an Issue.
type Severity string

// Severity values.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue codes: stable, machine-readable classes for pipeline tooling.
const (
	CodeUnknownField        = "unknown-field"
	CodeUnknownStep         = "unknown-step"
	CodeMissingRequired     = "missing-required"
	CodeBadType             = "bad-type"
	CodeBadEnum             = "bad-enum"
	CodeBadDuration         = "bad-duration"
	CodeBadTimestamp        = "bad-timestamp"
	CodeDuplicateKey        = "duplicate-key"
	CodeMissingFile         = "missing-file"
	CodePlatformUnsupported = "platform-unsupported"
	CodeSemantic            = "semantic"
)

// Issue is one validation finding.
type Issue struct {
	Severity Severity `json:"severity"`
	Code     string   `json:"code"`
	// Path is the logical location, e.g. "phases.install[0].with.windowStyle".
	Path    string `json:"path"`
	Pos     Pos    `json:"pos"`
	Message string `json:"message"`
}

// Report is the machine-readable result of validating one manifest.
type Report struct {
	Manifest string  `json:"manifest"`
	Valid    bool    `json:"valid"` // no error-severity issues
	Issues   []Issue `json:"issues"`
}

// NewReport assembles a Report from collected issues, sorting them by
// position then path.
func NewReport(manifestPath string, issues []Issue) Report {
	SortIssues(issues)
	valid := true
	for _, i := range issues {
		if i.Severity == SeverityError {
			valid = false
			break
		}
	}
	if issues == nil {
		issues = []Issue{}
	}
	return Report{Manifest: manifestPath, Valid: valid, Issues: issues}
}

// SortIssues orders issues by (line, column, path) for stable output.
func SortIssues(issues []Issue) {
	sort.SliceStable(issues, func(a, b int) bool {
		ia, ib := issues[a], issues[b]
		if ia.Pos.Line != ib.Pos.Line {
			return ia.Pos.Line < ib.Pos.Line
		}
		if ia.Pos.Col != ib.Pos.Col {
			return ia.Pos.Col < ib.Pos.Col
		}
		return ia.Path < ib.Path
	})
}

// HasErrors reports whether any issue is error-severity.
func HasErrors(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == SeverityError {
			return true
		}
	}
	return false
}
