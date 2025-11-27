package httpclient

import (
	"net/http"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/stream"
)

type StreamMeta struct {
	Status         string
	StatusCode     int
	Proto          string
	Headers        http.Header
	RequestHeaders http.Header
	RequestMethod  string
	RequestHost    string
	RequestLength  int64
	RequestTE      []string
	EffectiveURL   string
	ConnectedAt    time.Time
	Request        *restfile.Request
	BaseDir        string
}

type StreamHandle struct {
	Session *stream.Session
	Meta    StreamMeta
}
