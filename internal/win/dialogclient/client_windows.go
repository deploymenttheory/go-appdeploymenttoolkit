//go:build windows

package dialogclient

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/dialogserver"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/ipc"
	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/win/winerr"
)

// handles parses the pipe-handle strings into raw handle values.
func (c Config) handles() (in, out uint64, err error) {
	in, err = strconv.ParseUint(c.PipeIn, 10, 64)
	if err != nil {
		return 0, 0, winerr.Wrap("dialogclient: bad --pipe-in", winerr.ErrInvalidOption)
	}
	out, err = strconv.ParseUint(c.PipeOut, 10, 64)
	if err != nil {
		return 0, 0, winerr.Wrap("dialogclient: bad --pipe-out", winerr.ErrInvalidOption)
	}
	if in == 0 || out == 0 {
		return 0, 0, winerr.Wrap(
			"dialogclient: --pipe-in and --pipe-out are required",
			winerr.ErrInvalidOption,
		)
	}
	return in, out, nil
}

// ack is the non-nil result returned for commands that carry no reply body; it
// keeps the ipc handler from returning a bare (nil, nil).
type ack struct{}

// newHandler builds the ipc.Handler that dispatches protocol commands to the
// renderer.
func newHandler(r dialogserver.Renderer) ipc.Handler {
	return func(ctx context.Context, req *ipc.Request) (any, error) {
		switch req.Command {
		case ipc.CmdShowModalDialog:
			var p ipc.ModalDialogPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return r.ShowModal(ctx, p)
		case ipc.CmdShowProgressDialog:
			var p ipc.ProgressPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return ack{}, r.ShowProgress(ctx, p)
		case ipc.CmdUpdateProgress:
			var p ipc.ProgressPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return ack{}, r.UpdateProgress(ctx, p)
		case ipc.CmdCloseProgress:
			return ack{}, r.CloseProgress(ctx)
		case ipc.CmdShowBalloonTip:
			var p ipc.BalloonPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return ack{}, r.ShowBalloon(ctx, p)
		case ipc.CmdPromptToCloseApps:
			var p ipc.PromptToCloseAppsPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return r.PromptToCloseApps(ctx, p)
		case ipc.CmdMinimizeWindows:
			return ack{}, r.MinimizeWindows(ctx)
		case ipc.CmdSendKeys:
			var p ipc.SendKeysPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return ack{}, r.SendKeys(ctx, p)
		case ipc.CmdGetWindowInfo:
			var p ipc.SendKeysPayload
			if err := decode(req.Payload, &p); err != nil {
				return nil, err
			}
			return r.GetWindowInfo(ctx, p)
		case ipc.CmdRefreshDesktop:
			return ack{}, r.RefreshDesktop(ctx)
		default:
			return nil, winerr.Wrap(
				"dialogclient: unsupported command "+string(req.Command),
				winerr.ErrInvalidOption,
			)
		}
	}
}

// decode unmarshals a command payload, wrapping malformed bodies.
func decode(raw json.RawMessage, v any) error {
	if len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("dialogclient: decoding payload: %w", err)
	}
	return nil
}

// ClientMain is the `client` subcommand entrypoint the deployment side re-execs
// into the interactive user session. It wraps the two inherited pipe handles
// (cfg.PipeIn/PipeOut) as a duplex stream and serves dialog commands against a
// Windows renderer until the server closes the connection.
//
// Phase 4's cmd/adt parses the --pipe-in/--pipe-out/--nonce flags and passes
// them in through Config.
func ClientMain(ctx context.Context, cfg Config) error {
	inHandle, outHandle, err := cfg.handles()
	if err != nil {
		return err
	}

	in := os.NewFile(uintptr(inHandle), "dialog-pipe-in")
	out := os.NewFile(uintptr(outHandle), "dialog-pipe-out")
	if in == nil || out == nil {
		return fmt.Errorf("dialogclient: invalid pipe handles: %w", os.ErrInvalid)
	}
	transport := &clientPipe{r: in, w: out}
	defer func() { _ = transport.Close() }()

	renderer := NewRenderer()
	defer renderer.Close()

	if err := ipc.Serve(ctx, transport, cfg.Nonce, newHandler(renderer)); err != nil {
		return err
	}
	return nil
}

// clientPipe adapts the client's two inherited half-pipes to a single
// io.ReadWriteCloser for ipc.Serve.
type clientPipe struct {
	r *os.File
	w *os.File
}

func (c *clientPipe) Read(p []byte) (int, error)  { return c.r.Read(p) }
func (c *clientPipe) Write(p []byte) (int, error) { return c.w.Write(p) }

func (c *clientPipe) Close() error {
	rerr := c.r.Close()
	werr := c.w.Close()
	if rerr != nil {
		return fmt.Errorf("dialogclient: closing pipe: %w", rerr)
	}
	if werr != nil {
		return fmt.Errorf("dialogclient: closing pipe: %w", werr)
	}
	return nil
}
