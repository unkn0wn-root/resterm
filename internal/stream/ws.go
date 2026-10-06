package stream

// WSType names a WebSocket frame the way transcripts show it. It is the
// opcode's name, except that a text frame sent as JSON is "json".
type WSType string

const (
	WSText   WSType = "text"
	WSJSON   WSType = "json"
	WSBinary WSType = "binary"
	WSPing   WSType = "ping"
	WSPong   WSType = "pong"
	WSClose  WSType = "close"
)

// WSClosedBy names what ended a WebSocket session.
type WSClosedBy string

const (
	WSClosedByServer   WSClosedBy = "server"
	WSClosedByClient   WSClosedBy = "client"
	WSClosedByTimeout  WSClosedBy = "timeout"
	WSClosedByCanceled WSClosedBy = "canceled"
	WSClosedByError    WSClosedBy = "error"
)

// WSCloseCode is a close status code from RFC 6455 section 7.4.
type WSCloseCode int

const WSCloseNormal WSCloseCode = 1000

type WSMetadata struct {
	Type WSType
	// Step labels what sent the frame: a @ws step, the console, or the close
	// resterm sends after the last step.
	Step string
	// ClosedBy, Code and Reason describe how the session ended. Code is set
	// only on a close frame.
	ClosedBy WSClosedBy
	Code     WSCloseCode
	Reason   string
}
