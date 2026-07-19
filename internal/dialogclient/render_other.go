//go:build !windows

package dialogclient

import (
	"context"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Renderer is the non-Windows placeholder. Every method reports ErrNotWindows;
// the toolkit never constructs a UI on non-Windows platforms.
type Renderer struct{}

// NewRenderer returns the stub renderer.
func NewRenderer() *Renderer { return &Renderer{} }

// compile-time proof the stub satisfies the server-side contract.
var _ dialogserver.Renderer = (*Renderer)(nil)

func notWindows() error {
	return winerr.Wrap("dialogclient: user interface requires Windows", winerr.ErrNotWindows)
}

// ShowModal reports ErrNotWindows.
func (*Renderer) ShowModal(
	_ context.Context,
	_ ipc.ModalDialogPayload,
) (ipc.ModalDialogResult, error) {
	return ipc.ModalDialogResult{}, notWindows()
}

// ShowProgress reports ErrNotWindows.
func (*Renderer) ShowProgress(
	_ context.Context,
	_ ipc.ProgressPayload,
) error {
	return notWindows()
}

// UpdateProgress reports ErrNotWindows.
func (*Renderer) UpdateProgress(
	_ context.Context,
	_ ipc.ProgressPayload,
) error {
	return notWindows()
}

// CloseProgress reports ErrNotWindows.
func (*Renderer) CloseProgress(_ context.Context) error { return notWindows() }

// ShowBalloon reports ErrNotWindows.
func (*Renderer) ShowBalloon(_ context.Context, _ ipc.BalloonPayload) error { return notWindows() }

// MinimizeWindows reports ErrNotWindows.
func (*Renderer) MinimizeWindows(_ context.Context) error { return notWindows() }

// SendKeys reports ErrNotWindows.
func (*Renderer) SendKeys(_ context.Context, _ ipc.SendKeysPayload) error { return notWindows() }

// GetWindowInfo reports ErrNotWindows.
func (*Renderer) GetWindowInfo(
	_ context.Context,
	_ ipc.SendKeysPayload,
) (ipc.WindowInfoResult, error) {
	return ipc.WindowInfoResult{}, notWindows()
}

// RefreshDesktop reports ErrNotWindows.
func (*Renderer) RefreshDesktop(_ context.Context) error { return notWindows() }

// Close is a no-op for the stub.
func (*Renderer) Close() {}
