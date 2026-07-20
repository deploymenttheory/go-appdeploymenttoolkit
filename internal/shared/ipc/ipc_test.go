package ipc

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteReadMessageRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	req := Request{ID: 7, Command: CmdShowBalloonTip}
	require.NoError(t, WriteMessage(&buf, &req))

	var got Request
	require.NoError(t, ReadMessage(bufio.NewReader(&buf), &got))
	assert.Equal(t, req, got)
}

func TestReadMessageRejectsOversizeFrame(t *testing.T) {
	var buf bytes.Buffer
	// Length prefix claiming 9 MiB, exceeding maxFrame.
	buf.Write([]byte{0x00, 0x90, 0x00, 0x00})
	err := ReadMessage(bufio.NewReader(&buf), &Request{})
	assert.Error(t, err)
}

// echoHandler answers modal dialogs with a fixed button and echoes input.
func echoHandler(_ context.Context, req *Request) (any, error) {
	switch req.Command {
	case CmdShowModalDialog:
		var p ModalDialogPayload
		if err := json.Unmarshal(req.Payload, &p); err != nil {
			return nil, err
		}
		res := ModalDialogResult{Button: "Continue"}
		if p.DialogType == DialogInput && p.Input != nil {
			res.Button = "Right"
			res.Input = p.Input.DefaultValue
		}
		return res, nil
	default:
		return nil, nil
	}
}

func TestServerClientRoundTripOverPipe(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- Serve(ctx, clientEnd, "secret-nonce", echoHandler) }()

	conn := NewServerConn(serverEnd)
	require.NoError(t, conn.Handshake("secret-nonce"))

	payload, err := json.Marshal(ModalDialogPayload{
		DialogType: DialogInput,
		Input:      &InputOptions{DefaultValue: "hello"},
	})
	require.NoError(t, err)
	resp, err := conn.Do(&Request{Command: CmdShowModalDialog, Payload: payload})
	require.NoError(t, err)
	require.True(t, resp.OK, resp.Error)

	var result ModalDialogResult
	require.NoError(t, json.Unmarshal(resp.Result, &result))
	assert.Equal(t, "Right", result.Button)
	assert.Equal(t, "hello", result.Input)

	conn.Close()
	_ = serverEnd.Close()
	select {
	case err := <-serveErr:
		assert.NoError(t, err)
	case <-ctx.Done():
		t.Fatal("serve loop did not exit")
	}
}

func TestHandshakeRejectsWrongNonce(t *testing.T) {
	serverEnd, clientEnd := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Serve(ctx, clientEnd, "right-nonce", echoHandler) }()

	conn := NewServerConn(serverEnd)
	err := conn.Handshake("wrong-nonce")
	assert.Error(t, err)
}
