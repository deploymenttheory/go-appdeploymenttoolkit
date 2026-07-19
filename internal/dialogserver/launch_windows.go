//go:build windows

package dialogserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/deploymenttheory/go-appdeploymenttoolkit/internal/ipc"
	win32 "github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/security"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/environment"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/pipes"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/remotedesktop"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/threading"
)

// Launch spawns the client renderer in the target user's session and returns a
// DialogServer that drives it over a pair of anonymous pipes.
//
// The wiring, top to bottom:
//   - two anonymous pipes are created with inheritable ends: s2c carries
//     server->client frames, c2s carries client->server frames;
//   - the server keeps s2cWrite and c2sRead (marked non-inheritable) and wraps
//     them as one duplex io.ReadWriteCloser;
//   - the client inherits s2cRead (its input) and c2sWrite (its output); their
//     numeric handle values (preserved across inheritance) are passed on the
//     command line as --pipe-in/--pipe-out alongside a random --nonce;
//   - the same executable is re-executed as `<exe> client ...` via
//     CreateProcessAsUser in the user's session (the token/env sequence mirrors
//     internal/procmgmt asuser_windows.go, but with bInheritHandles=true so the
//     two client pipe ends cross into the child);
//   - the server performs the ipc hello handshake and hands the duplex to a
//     pipe-backed Renderer.
//
// This function cannot be exercised off Windows; it is a thin, heavily
// commented marshaling layer with no independently testable logic.
func Launch(_ context.Context, cfg LaunchConfig) (*DialogServer, error) {
	exe := cfg.Executable
	if exe == "" {
		self, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("dialogserver: resolving executable: %w", err)
		}
		exe = self
	}
	sub := cfg.ClientSubcommand
	if sub == "" {
		sub = "client"
	}

	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, fmt.Errorf("dialogserver: generating nonce: %w", err)
	}
	nonce := hex.EncodeToString(nonceBytes)

	s2cRead, s2cWrite, err := makePipe()
	if err != nil {
		return nil, err
	}
	c2sRead, c2sWrite, err := makePipe()
	if err != nil {
		_ = windows.CloseHandle(s2cRead)
		_ = windows.CloseHandle(s2cWrite)
		return nil, err
	}

	// The server keeps s2cWrite (it writes requests) and c2sRead (it reads
	// responses); those must not leak into the child.
	_ = windows.SetHandleInformation(s2cWrite, windows.HANDLE_FLAG_INHERIT, 0)
	_ = windows.SetHandleInformation(c2sRead, windows.HANDLE_FLAG_INHERIT, 0)
	// The child inherits s2cRead (its input) and c2sWrite (its output).
	_ = windows.SetHandleInformation(
		s2cRead,
		windows.HANDLE_FLAG_INHERIT,
		windows.HANDLE_FLAG_INHERIT,
	)
	_ = windows.SetHandleInformation(
		c2sWrite,
		windows.HANDLE_FLAG_INHERIT,
		windows.HANDLE_FLAG_INHERIT,
	)

	args := fmt.Sprintf("%s %s --pipe-in %d --pipe-out %d --nonce %s",
		windows.EscapeArg(exe), sub, uint64(s2cRead), uint64(c2sWrite), nonce)

	proc, err := createClientProcess(cfg.Session.SessionID, exe, args)
	if err != nil {
		closeAll(s2cRead, s2cWrite, c2sRead, c2sWrite)
		return nil, err
	}
	// The child now owns its ends; the parent drops its copies so EOF flows.
	_ = windows.CloseHandle(s2cRead)
	_ = windows.CloseHandle(c2sWrite)

	duplex := &duplexPipe{
		r:    os.NewFile(uintptr(c2sRead), "dialog-c2s"),
		w:    os.NewFile(uintptr(s2cWrite), "dialog-s2c"),
		proc: proc,
	}
	conn := ipc.NewServerConn(duplex)
	if err := conn.Handshake(nonce); err != nil {
		conn.Close()
		_ = duplex.Close()
		return nil, err
	}
	renderer := &reexecRenderer{Renderer: NewPipeRenderer(conn), transport: duplex}
	return New(renderer), nil
}

// makePipe creates one anonymous pipe whose two ends are both inheritable; the
// caller then narrows inheritance per end.
func makePipe() (read, write windows.Handle, err error) {
	sa := security.SECURITY_ATTRIBUTES{
		BInheritHandle: foundation.BOOL(win32.Bool32(true)),
	}
	sa.NLength = uint32(unsafe.Sizeof(sa))
	var hr, hw foundation.HANDLE
	if perr := pipes.CreatePipe(&hr, &hw, &sa, 0); perr != nil {
		return 0, 0, fmt.Errorf("dialogserver: CreatePipe: %w", perr)
	}
	return windows.Handle(hr), windows.Handle(hw), nil
}

// createClientProcess runs exe (with the given command line) as the user who
// owns sessionID, inheriting the marked pipe handles. It mirrors the token and
// environment-block sequence used by internal/procmgmt.LaunchAsUser, differing
// only in that bInheritHandles is true.
func createClientProcess(sessionID uint32, exe, cmdLine string) (windows.Handle, error) {
	var userToken foundation.HANDLE
	if err := remotedesktop.WTSQueryUserToken(sessionID, &userToken); err != nil {
		return 0, fmt.Errorf("dialogserver: WTSQueryUserToken(session %d): %w", sessionID, err)
	}
	defer func() { _ = windows.CloseHandle(windows.Handle(userToken)) }()

	var primary windows.Token
	if err := windows.DuplicateTokenEx(
		windows.Token(userToken),
		windows.MAXIMUM_ALLOWED,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&primary,
	); err != nil {
		return 0, fmt.Errorf("dialogserver: DuplicateTokenEx: %w", err)
	}
	defer func() { _ = primary.Close() }()

	var envBlock unsafe.Pointer
	if err := environment.CreateEnvironmentBlock(
		&envBlock,
		foundation.HANDLE(primary),
		false,
	); err != nil {
		return 0, fmt.Errorf("dialogserver: CreateEnvironmentBlock: %w", err)
	}
	defer func() { _ = environment.DestroyEnvironmentBlock(envBlock) }()

	cmdBuf, err := windows.UTF16FromString(cmdLine)
	if err != nil {
		return 0, fmt.Errorf("dialogserver: encoding command line: %w", err)
	}

	si := threading.STARTUPINFOW{
		LpDesktop: foundation.PWSTR(win32.UTF16Ptr(`winsta0\default`)),
	}
	si.Cb = uint32(unsafe.Sizeof(si))

	var pi threading.PROCESS_INFORMATION
	if err := threading.CreateProcessAsUser(
		foundation.HANDLE(primary),
		exe,
		foundation.PWSTR(unsafe.SliceData(cmdBuf)),
		nil,  // process attributes
		nil,  // thread attributes
		true, // bInheritHandles: the marked pipe ends cross into the child
		threading.CREATE_UNICODE_ENVIRONMENT|threading.CREATE_NO_WINDOW,
		envBlock,
		filepath.Dir(exe),
		&si,
		&pi,
	); err != nil {
		return 0, fmt.Errorf("dialogserver: CreateProcessAsUser(%q): %w", exe, err)
	}
	_ = windows.CloseHandle(windows.Handle(pi.HThread))
	return windows.Handle(pi.HProcess), nil
}

// closeAll closes a set of handles, ignoring errors (cleanup path).
func closeAll(handles ...windows.Handle) {
	for _, h := range handles {
		_ = windows.CloseHandle(h)
	}
}

// duplexPipe presents the server's two half-pipes as one io.ReadWriteCloser and
// owns the client process handle for teardown.
type duplexPipe struct {
	r    *os.File
	w    *os.File
	proc windows.Handle
}

func (d *duplexPipe) Read(p []byte) (int, error)  { return d.r.Read(p) }
func (d *duplexPipe) Write(p []byte) (int, error) { return d.w.Write(p) }

func (d *duplexPipe) Close() error {
	rerr := d.r.Close()
	werr := d.w.Close()
	if d.proc != 0 {
		_ = windows.CloseHandle(d.proc)
		d.proc = 0
	}
	if rerr != nil {
		return fmt.Errorf("dialogserver: closing pipe: %w", rerr)
	}
	if werr != nil {
		return fmt.Errorf("dialogserver: closing pipe: %w", werr)
	}
	return nil
}

// reexecRenderer decorates the pipe-backed Renderer so Close also tears down
// the transport (which signals EOF to the client and lets it exit).
type reexecRenderer struct {
	Renderer
	transport *duplexPipe
}

func (r *reexecRenderer) Close() {
	r.Renderer.Close()      // best-effort ipc Close frame
	_ = r.transport.Close() // drop the pipes and the process handle
}
