// Package ipc defines the length-prefixed JSON protocol that carries dialog
// commands between the deployment process (server, possibly running as
// SYSTEM) and the client process that renders UI in the interactive user
// session. The wire format and message types are portable and unit-tested;
// the pipe transport lives in transport.go alongside a net.Pipe-backed test.
package ipc

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/winerr"
)

// Command identifies a dialog operation the server asks the client to perform.
// The set mirrors PSADT's ServerInstance payload commands.
type Command string

// Command values.
const (
	CmdHello              Command = "Hello"
	CmdShowModalDialog    Command = "ShowModalDialog"
	CmdShowProgressDialog Command = "ShowProgressDialog"
	CmdUpdateProgress     Command = "UpdateProgressDialog"
	CmdCloseProgress      Command = "CloseProgressDialog"
	CmdShowBalloonTip     Command = "ShowBalloonTip"
	CmdMinimizeWindows    Command = "MinimizeAllWindows"
	CmdSendKeys           Command = "SendKeys"
	CmdGetWindowInfo      Command = "GetProcessWindowInfo"
	CmdRefreshDesktop     Command = "RefreshDesktopAndEnvironmentVariables"
	CmdClose              Command = "Close"
)

// DialogType identifies the modal dialog kind for CmdShowModalDialog.
type DialogType string

// DialogType values.
const (
	DialogCloseApps     DialogType = "CloseApps"
	DialogCustom        DialogType = "Custom"
	DialogInput         DialogType = "Input"
	DialogListSelection DialogType = "ListSelection"
	DialogRestart       DialogType = "Restart"
	DialogBox           DialogType = "DialogBox"
)

// Request is one server→client message. Payload is the command-specific body
// (e.g. ModalDialogPayload); it is raw JSON so the transport stays generic.
type Request struct {
	ID      uint64          `json:"id"`
	Command Command         `json:"command"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Response is one client→server reply.
type Response struct {
	ID     uint64          `json:"id"`
	OK     bool            `json:"ok"`
	Error  string          `json:"error,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
}

// HelloPayload is exchanged on connect; Nonce guards against handle confusion
// (the server hands the client the nonce out-of-band via the command line).
type HelloPayload struct {
	Nonce         string `json:"nonce"`
	ProtocolMajor int    `json:"protocolMajor"`
}

// ProtocolMajor is the wire-compatibility version.
const ProtocolMajor = 1

// maxFrame bounds a single message to guard against corrupt length prefixes.
const maxFrame = 8 << 20 // 8 MiB

// WriteMessage frames v as a 4-byte big-endian length prefix followed by its
// JSON encoding.
func WriteMessage(w io.Writer, v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("ipc: marshaling message: %w", err)
	}
	if len(body) > maxFrame {
		return winerr.Wrap("ipc: message exceeds maximum frame size", winerr.ErrInvalidOption)
	}
	var prefix [4]byte
	//#nosec G115 -- len(body) is bounded by the maxFrame check above
	binary.BigEndian.PutUint32(prefix[:], uint32(len(body)))
	if _, err := w.Write(prefix[:]); err != nil {
		return fmt.Errorf("ipc: writing length prefix: %w", err)
	}
	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("ipc: writing message body: %w", err)
	}
	return nil
}

// ReadMessage reads one length-prefixed frame and unmarshals it into v.
func ReadMessage(r *bufio.Reader, v any) error {
	var prefix [4]byte
	if _, err := io.ReadFull(r, prefix[:]); err != nil {
		return fmt.Errorf("ipc: reading length prefix: %w", err)
	}
	n := binary.BigEndian.Uint32(prefix[:])
	if n > maxFrame {
		return winerr.Wrap("ipc: frame exceeds maximum size", winerr.ErrInvalidOption)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return fmt.Errorf("ipc: reading message body: %w", err)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("ipc: unmarshaling message: %w", err)
	}
	return nil
}
