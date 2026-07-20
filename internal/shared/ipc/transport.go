package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/shared/errs"
)

// ServerConn is the server end of the dialog protocol: it sends requests and
// waits for the matching responses over a duplex stream.
type ServerConn struct {
	w      io.Writer
	r      *bufio.Reader
	mu     sync.Mutex // serializes request/response round-trips
	nextID atomic.Uint64
}

// NewServerConn wraps a duplex stream (one pipe pair) as the server end.
func NewServerConn(rw io.ReadWriteCloser) *ServerConn {
	return &ServerConn{w: rw, r: bufio.NewReader(rw)}
}

// Handshake sends the hello frame and verifies the client's protocol version
// and nonce echo.
func (c *ServerConn) Handshake(nonce string) error {
	payload, err := json.Marshal(HelloPayload{Nonce: nonce, ProtocolMajor: ProtocolMajor})
	if err != nil {
		return fmt.Errorf("ipc: marshaling hello: %w", err)
	}
	resp, err := c.Do(&Request{Command: CmdHello, Payload: payload})
	if err != nil {
		return err
	}
	if !resp.OK {
		return errs.Wrap(
			"ipc: client rejected handshake: "+resp.Error,
			errs.ErrDialogUnavailable,
		)
	}
	return nil
}

// Do sends req (assigning it an ID) and returns the matching response. Calls
// are serialized so the single stream carries one round-trip at a time.
func (c *ServerConn) Do(req *Request) (*Response, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	req.ID = c.nextID.Add(1)
	if err := WriteMessage(c.w, req); err != nil {
		return nil, err
	}
	var resp Response
	if err := ReadMessage(c.r, &resp); err != nil {
		return nil, err
	}
	if resp.ID != req.ID {
		return nil, errs.Wrap(
			fmt.Sprintf("ipc: response id %d does not match request %d", resp.ID, req.ID),
			errs.ErrDialogUnavailable,
		)
	}
	return &resp, nil
}

// Close sends a best-effort Close command. It does not await a response: the
// client's Serve loop exits on Close without replying.
func (c *ServerConn) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	_ = WriteMessage(c.w, &Request{ID: c.nextID.Add(1), Command: CmdClose})
}

// Handler renders one request and returns its result body (or an error).
type Handler func(ctx context.Context, req *Request) (result any, err error)

// ackResult is the empty acknowledgement body returned for commands that
// succeed without a dialog result (e.g. the hello handshake).
type ackResult struct {
	OK bool `json:"ok"`
}

// Serve runs the client-side loop: read requests, dispatch to handler, write
// responses, until the stream closes or a Close command arrives. The hello
// handshake is validated against expectedNonce before any dialog is handled.
func Serve(
	ctx context.Context,
	rw io.ReadWriteCloser,
	expectedNonce string,
	handler Handler,
) error {
	r := bufio.NewReader(rw)
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("ipc: %w", err)
		}
		var req Request
		if err := ReadMessage(r, &req); err != nil {
			if errors.Is(err, io.EOF) || isClosed(err) {
				return nil
			}
			return err
		}
		if req.Command == CmdClose {
			return nil
		}
		result, herr := dispatch(ctx, &req, expectedNonce, handler)
		resp := Response{ID: req.ID, OK: herr == nil}
		if herr != nil {
			resp.Error = herr.Error()
		} else if result != nil {
			body, err := json.Marshal(result)
			if err != nil {
				resp.OK, resp.Error = false, err.Error()
			} else {
				resp.Result = body
			}
		}
		if err := WriteMessage(rw, &resp); err != nil {
			return err
		}
	}
}

func dispatch(
	ctx context.Context,
	req *Request,
	expectedNonce string,
	handler Handler,
) (any, error) {
	if req.Command == CmdHello {
		var hello HelloPayload
		if err := json.Unmarshal(req.Payload, &hello); err != nil {
			return nil, fmt.Errorf("ipc: bad hello payload: %w", err)
		}
		if hello.ProtocolMajor != ProtocolMajor {
			return nil, errs.Wrap("ipc: protocol version mismatch", errs.ErrDialogUnavailable)
		}
		if expectedNonce != "" && hello.Nonce != expectedNonce {
			return nil, errs.Wrap("ipc: nonce mismatch", errs.ErrDialogUnavailable)
		}
		return ackResult{OK: true}, nil // accepted handshake, no dialog result body
	}
	return handler(ctx, req)
}

func isClosed(err error) bool {
	return err != nil && (errors.Is(err, io.ErrClosedPipe) || errors.Is(err, io.ErrUnexpectedEOF))
}
