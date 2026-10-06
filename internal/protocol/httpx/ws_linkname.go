//go:build !js

package httpx

import (
	"context"

	_ "unsafe"

	"github.com/coder/websocket"
)

// this wraps the unexported (*websocket.Conn).writeControl method via go:linkname.
// This allows us to emit control frames such as pong while reusing lib's framing logic.
//
//go:linkname wsWriteControl github.com/coder/websocket.(*Conn).writeControl
func wsWriteControl(conn *websocket.Conn, ctx context.Context, opcode int, payload []byte) error
