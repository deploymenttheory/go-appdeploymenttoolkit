package dialogclient

// Config carries the pipe handles and nonce the deployment side passes to the
// re-execed client. The values arrive as the command-line strings the launcher
// wrote (--pipe-in/--pipe-out are decimal inherited handle numbers).
//
// It is portable so the cmd/adt front end can build the client subcommand on
// every platform; the handles are only wired up in the Windows ClientMain.
type Config struct {
	PipeIn  string // inherited handle the client reads requests from
	PipeOut string // inherited handle the client writes responses to
	Nonce   string // handshake nonce echoed to the server
}
