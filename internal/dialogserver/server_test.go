package dialogserver

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
)

// fakeRenderer records the calls the server routes to it and returns canned
// results. It documents the contract: the server forwards without inspecting
// payloads, and silent/non-interactive short-circuiting is the caller's job.
type fakeRenderer struct {
	mu       sync.Mutex
	modal    ipc.ModalDialogPayload
	progress []ipc.ProgressPayload
	balloon  ipc.BalloonPayload
	closed   int
	closedP  int
	result   ipc.ModalDialogResult
	winInfo  ipc.WindowInfoResult
}

func (f *fakeRenderer) ShowModal(_ context.Context, p ipc.ModalDialogPayload) (ipc.ModalDialogResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.modal = p
	return f.result, nil
}

func (f *fakeRenderer) ShowProgress(_ context.Context, p ipc.ProgressPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, p)
	return nil
}

func (f *fakeRenderer) UpdateProgress(_ context.Context, p ipc.ProgressPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.progress = append(f.progress, p)
	return nil
}

func (f *fakeRenderer) CloseProgress(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closedP++
	return nil
}

func (f *fakeRenderer) ShowBalloon(_ context.Context, p ipc.BalloonPayload) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.balloon = p
	return nil
}

func (f *fakeRenderer) MinimizeWindows(_ context.Context) error { return nil }

func (f *fakeRenderer) SendKeys(_ context.Context, _ ipc.SendKeysPayload) error { return nil }

func (f *fakeRenderer) GetWindowInfo(_ context.Context, _ ipc.SendKeysPayload) (ipc.WindowInfoResult, error) {
	return f.winInfo, nil
}

func (f *fakeRenderer) RefreshDesktop(_ context.Context) error { return nil }

func (f *fakeRenderer) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func TestServerRoutesToRenderer(t *testing.T) {
	f := &fakeRenderer{result: ipc.ModalDialogResult{Button: "Continue"}}
	srv := New(f)
	ctx := context.Background()

	res, err := srv.ShowModal(ctx, ipc.ModalDialogPayload{DialogType: ipc.DialogCloseApps})
	require.NoError(t, err)
	assert.Equal(t, "Continue", res.Button)
	assert.Equal(t, ipc.DialogCloseApps, f.modal.DialogType)

	require.NoError(t, srv.ShowProgress(ctx, ipc.ProgressPayload{StatusMessage: "one"}))
	require.NoError(t, srv.UpdateProgress(ctx, ipc.ProgressPayload{StatusMessage: "two"}))
	require.NoError(t, srv.CloseProgress(ctx))
	require.Len(t, f.progress, 2)
	assert.Equal(t, 1, f.closedP)

	require.NoError(t, srv.ShowBalloon(ctx, ipc.BalloonPayload{Text: "hi"}))
	assert.Equal(t, "hi", f.balloon.Text)
}

func TestServerRejectsAfterClose(t *testing.T) {
	f := &fakeRenderer{}
	srv := New(f)
	srv.Close()
	srv.Close() // idempotent
	assert.Equal(t, 1, f.closed)

	_, err := srv.ShowModal(context.Background(), ipc.ModalDialogPayload{})
	assert.Error(t, err)
	assert.Error(t, srv.ShowProgress(context.Background(), ipc.ProgressPayload{}))
}

// pipeHandler is a client-side ipc handler that answers dialog commands, used
// to prove the pipe-backed renderer works end-to-end over net.Pipe.
func pipeHandler(_ context.Context, req *ipc.Request) (any, error) {
	switch req.Command {
	case ipc.CmdShowModalDialog:
		var p ipc.ModalDialogPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return nil, err
		}
		res := ipc.ModalDialogResult{Button: "Defer"}
		if p.Input != nil {
			res.Button = "Right"
			res.Input = p.Input.DefaultValue
		}
		return res, nil
	case ipc.CmdGetWindowInfo:
		return ipc.WindowInfoResult{Windows: []ipc.WindowInfo{{Handle: 42, Title: "Notepad"}}}, nil
	default:
		return nil, nil
	}
}

func TestPipeRendererEndToEnd(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- ipc.Serve(ctx, clientEnd, "nonce", pipeHandler) }()

	conn := ipc.NewServerConn(serverEnd)
	require.NoError(t, conn.Handshake("nonce"))

	srv := New(NewPipeRenderer(conn))

	res, err := srv.ShowModal(ctx, ipc.ModalDialogPayload{
		DialogType: ipc.DialogInput,
		Input:      &ipc.InputOptions{DefaultValue: "hello"},
	})
	require.NoError(t, err)
	assert.Equal(t, "Right", res.Button)
	assert.Equal(t, "hello", res.Input)

	// A command with no result body must not error.
	require.NoError(t, srv.ShowProgress(ctx, ipc.ProgressPayload{StatusMessage: "go"}))

	info, err := srv.GetWindowInfo(ctx, ipc.SendKeysPayload{WindowTitle: "Note"})
	require.NoError(t, err)
	require.Len(t, info.Windows, 1)
	assert.Equal(t, "Notepad", info.Windows[0].Title)

	srv.Close()
	_ = serverEnd.Close()
	select {
	case err := <-serveErr:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("serve loop did not exit")
	}
}
