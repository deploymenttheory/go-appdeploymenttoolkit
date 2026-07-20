package macadt

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/deploy"
)

// Session is the deployment session (deploy.Session).
type Session = deploy.Session

// SessionOptions mirrors the session metadata a deployment declares.
type SessionOptions = deploy.SessionOptions

// OpenSession opens a deployment session on the shared engine. On darwin the
// Windows-only Auto-mode probes and deferral registry backend are inert; the
// session lifecycle, logging and exit-code semantics are fully functional.
func OpenSession(ctx context.Context, opts SessionOptions) (*Session, error) {
	return deploy.Open(ctx, opts)
}

// CloseSession finalizes a session and returns the process exit code.
func CloseSession(ctx context.Context, s *Session) int {
	return deploy.Close(ctx, s)
}

// CurrentSession returns the most recently opened active session.
func CurrentSession() (*Session, error) {
	return deploy.Current()
}
