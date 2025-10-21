//go:build !js

package httpclient

import (
	"context"

	_ "unsafe"

	"nhooyr.io/websocket"
)

// this wraps the unexported (*websocket.Conn).writeControl method via go:linkname.
// This allows us to emit control frames such as pong while reusing lib's framing logic.
//
//go:linkname wsWriteControl nhooyr.io/websocket.(*Conn).writeControl
func wsWriteControl(conn *websocket.Conn, ctx context.Context, opcode int, payload []byte) error
