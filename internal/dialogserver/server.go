// Package dialogserver is the deployment-side driver for the toolkit's user
// interface. It exposes a small Renderer interface that the psadt facade calls
// uniformly, backed either by an in-process renderer (when the deployment is
// already running interactively in the target user's session) or by a
// pipe-backed renderer that marshals each call to a client process launched in
// the user session (when the deployment runs as SYSTEM or in another session).
//
// The wire marshaling and the Renderer contract are portable and unit-tested
// against a net.Pipe-backed ipc.Serve loop; the re-exec plumbing that spawns
// the client lives in launch_windows.go.
package dialogserver

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/wts"
)

// LaunchConfig configures the re-exec path that spawns the client renderer in
// the target user's session (see Launch in launch_windows.go).
type LaunchConfig struct {
	// Session is the interactive user session the client is launched into.
	Session wts.SessionInfo
	// Executable is the program to re-exec as the client; empty uses the
	// running executable (os.Executable).
	Executable string
	// ClientSubcommand is the argv[1] the client entrypoint dispatches on;
	// empty defaults to "client".
	ClientSubcommand string
}

// Renderer renders the toolkit's dialogs. The deployment side calls it without
// caring whether the implementation draws locally (WebView2 in this process)
// or forwards each request over a pipe to a client process in the interactive
// session.
//
// Silent- and non-interactive-mode short-circuiting is the caller's
// responsibility (see psadt/dialogs.go): the server never suppresses a dialog
// on its own, it renders whatever it is asked to render.
type Renderer interface {
	// ShowModal renders a modal dialog and blocks until the user answers, the
	// dialog times out (Button == "Timeout") or is dismissed.
	ShowModal(ctx context.Context, p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error)
	// ShowProgress opens the modeless progress window.
	ShowProgress(ctx context.Context, p ipc.ProgressPayload) error
	// UpdateProgress updates the open progress window's message and bar.
	UpdateProgress(ctx context.Context, p ipc.ProgressPayload) error
	// CloseProgress closes the progress window.
	CloseProgress(ctx context.Context) error
	// ShowBalloon shows a tray balloon / toast notification.
	ShowBalloon(ctx context.Context, p ipc.BalloonPayload) error
	// MinimizeWindows minimizes every top-level window on the desktop.
	MinimizeWindows(ctx context.Context) error
	// SendKeys sends a keystroke sequence to the window matching p.WindowTitle.
	SendKeys(ctx context.Context, p ipc.SendKeysPayload) error
	// GetWindowInfo enumerates top-level windows matching p.WindowTitle (empty
	// returns every titled window).
	GetWindowInfo(ctx context.Context, p ipc.SendKeysPayload) (ipc.WindowInfoResult, error)
	// RefreshDesktop refreshes the desktop and broadcasts an environment change.
	RefreshDesktop(ctx context.Context) error
	// Close releases the renderer's resources (closing the pipe / client).
	Close()
}

// DialogServer is the deployment-side handle the psadt facade talks to. It owns
// a single Renderer for the process lifetime.
//
// The lock guards only the renderer pointer and closed flag; it is never held
// while a render runs, so a blocking modal never freezes an unrelated command.
// The underlying transport (ipc.ServerConn.Do) already serializes concurrent
// round-trips, and the in-process renderer gives each window its own OS thread.
type DialogServer struct {
	mu       sync.Mutex
	renderer Renderer
	closed   bool
}

// New wraps an already-constructed Renderer (the in-process WebView2 renderer
// or, in tests, a fake). Launch builds the re-exec variant on Windows.
func New(r Renderer) *DialogServer {
	return &DialogServer{renderer: r}
}

// active returns the renderer if the server is still open.
func (s *DialogServer) active() (Renderer, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errClosed()
	}
	return s.renderer, nil
}

// ShowModal forwards to the renderer.
func (s *DialogServer) ShowModal(
	ctx context.Context,
	p ipc.ModalDialogPayload,
) (ipc.ModalDialogResult, error) {
	r, err := s.active()
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	return r.ShowModal(ctx, p)
}

// ShowProgress forwards to the renderer.
func (s *DialogServer) ShowProgress(ctx context.Context, p ipc.ProgressPayload) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.ShowProgress(ctx, p)
}

// UpdateProgress forwards to the renderer.
func (s *DialogServer) UpdateProgress(ctx context.Context, p ipc.ProgressPayload) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.UpdateProgress(ctx, p)
}

// CloseProgress forwards to the renderer.
func (s *DialogServer) CloseProgress(ctx context.Context) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.CloseProgress(ctx)
}

// ShowBalloon forwards to the renderer.
func (s *DialogServer) ShowBalloon(ctx context.Context, p ipc.BalloonPayload) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.ShowBalloon(ctx, p)
}

// MinimizeWindows forwards to the renderer.
func (s *DialogServer) MinimizeWindows(ctx context.Context) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.MinimizeWindows(ctx)
}

// SendKeys forwards to the renderer.
func (s *DialogServer) SendKeys(ctx context.Context, p ipc.SendKeysPayload) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.SendKeys(ctx, p)
}

// GetWindowInfo forwards to the renderer.
func (s *DialogServer) GetWindowInfo(
	ctx context.Context,
	p ipc.SendKeysPayload,
) (ipc.WindowInfoResult, error) {
	r, err := s.active()
	if err != nil {
		return ipc.WindowInfoResult{}, err
	}
	return r.GetWindowInfo(ctx, p)
}

// RefreshDesktop forwards to the renderer.
func (s *DialogServer) RefreshDesktop(ctx context.Context) error {
	r, err := s.active()
	if err != nil {
		return err
	}
	return r.RefreshDesktop(ctx)
}

// Close releases the underlying renderer. It is safe to call more than once.
func (s *DialogServer) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	s.closed = true
	s.renderer.Close()
}

// errClosed reports use of a closed server.
func errClosed() error {
	return winerr.Wrap("dialogserver: server is closed", winerr.ErrDialogUnavailable)
}

// pipeRenderer forwards Renderer calls over an ipc.ServerConn to a client
// process. It is the marshaling layer between the Renderer contract and the
// length-prefixed JSON protocol; it holds no Windows-specific code and is
// exercised end-to-end by server_test.go over net.Pipe.
type pipeRenderer struct {
	conn *ipc.ServerConn
}

// NewPipeRenderer wraps a handshaken ipc.ServerConn as a Renderer. The caller
// is responsible for having performed the hello handshake first.
func NewPipeRenderer(conn *ipc.ServerConn) Renderer {
	return &pipeRenderer{conn: conn}
}

// do sends a command with an optional payload and returns the raw result body.
func (r *pipeRenderer) do(
	ctx context.Context,
	cmd ipc.Command,
	payload any,
) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, winerr.Wrap("dialogserver", err)
	}
	req := &ipc.Request{Command: cmd}
	if payload != nil {
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, winerr.Wrap(
				"dialogserver: marshaling "+string(cmd),
				winerr.ErrInvalidOption,
			)
		}
		req.Payload = body
	}
	resp, err := r.conn.Do(req)
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, winerr.Wrap(
			"dialogserver: client reported: "+resp.Error,
			winerr.ErrDialogUnavailable,
		)
	}
	return resp.Result, nil
}

func (r *pipeRenderer) ShowModal(
	ctx context.Context,
	p ipc.ModalDialogPayload,
) (ipc.ModalDialogResult, error) {
	raw, err := r.do(ctx, ipc.CmdShowModalDialog, p)
	if err != nil {
		return ipc.ModalDialogResult{}, err
	}
	var res ipc.ModalDialogResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &res); err != nil {
			return ipc.ModalDialogResult{}, winerr.Wrap(
				"dialogserver: decoding modal result", winerr.ErrDialogUnavailable)
		}
	}
	return res, nil
}

func (r *pipeRenderer) ShowProgress(ctx context.Context, p ipc.ProgressPayload) error {
	_, err := r.do(ctx, ipc.CmdShowProgressDialog, p)
	return err
}

func (r *pipeRenderer) UpdateProgress(ctx context.Context, p ipc.ProgressPayload) error {
	_, err := r.do(ctx, ipc.CmdUpdateProgress, p)
	return err
}

func (r *pipeRenderer) CloseProgress(ctx context.Context) error {
	_, err := r.do(ctx, ipc.CmdCloseProgress, nil)
	return err
}

func (r *pipeRenderer) ShowBalloon(ctx context.Context, p ipc.BalloonPayload) error {
	_, err := r.do(ctx, ipc.CmdShowBalloonTip, p)
	return err
}

func (r *pipeRenderer) MinimizeWindows(ctx context.Context) error {
	_, err := r.do(ctx, ipc.CmdMinimizeWindows, nil)
	return err
}

func (r *pipeRenderer) SendKeys(ctx context.Context, p ipc.SendKeysPayload) error {
	_, err := r.do(ctx, ipc.CmdSendKeys, p)
	return err
}

func (r *pipeRenderer) GetWindowInfo(
	ctx context.Context,
	p ipc.SendKeysPayload,
) (ipc.WindowInfoResult, error) {
	raw, err := r.do(ctx, ipc.CmdGetWindowInfo, p)
	if err != nil {
		return ipc.WindowInfoResult{}, err
	}
	var res ipc.WindowInfoResult
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &res); err != nil {
			return ipc.WindowInfoResult{}, winerr.Wrap(
				"dialogserver: decoding window info", winerr.ErrDialogUnavailable)
		}
	}
	return res, nil
}

func (r *pipeRenderer) RefreshDesktop(ctx context.Context) error {
	_, err := r.do(ctx, ipc.CmdRefreshDesktop, nil)
	return err
}

func (r *pipeRenderer) Close() {
	r.conn.Close()
}
