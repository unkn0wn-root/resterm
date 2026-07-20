package mock

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// controlNamespace is reserved for resterm endpoints. Mocks cannot claim
// literal paths under it, so new control endpoints stay compatible.
const (
	controlNamespace = "/.resterm/"
	controlPrefix    = controlNamespace + "mock/v1"

	controlResetPath = controlPrefix + "/sequences/reset"
	controlClearPath = controlPrefix + "/journal/clear"
	controlCountPath = controlPrefix + "/requests/count"
	controlHeader    = "X-Resterm-Control"
	controlBodyLimit = 64 << 10
)

var controlRoutes = map[string]func(*Server, http.ResponseWriter, *http.Request){
	controlResetPath: (*Server).controlReset,
	controlClearPath: (*Server).controlClear,
	controlCountPath: (*Server).controlCount,
}

type resetRequest struct {
	Name string `json:"name,omitempty"`
}

type resetResponse struct {
	Reset int `json:"reset"`
}

type countResponse struct {
	Count uint64 `json:"count"`
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) bool {
	handle, ok := controlRoutes[r.URL.Path]
	if !ok {
		return false
	}
	if !controlRequestAllowed(r) {
		writeProblem(w, http.StatusForbidden, "mock control API is available only to local non-browser clients")
		return true
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeProblem(w, http.StatusMethodNotAllowed, "mock control API requires POST")
		return true
	}
	handle(s, w, r)
	return true
}

func (s *Server) controlReset(w http.ResponseWriter, r *http.Request) {
	var request resetRequest
	if err := decodeControlRequest(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	name := strings.TrimSpace(request.Name)
	if name != "" && !restfile.ValidMockName(name) {
		writeProblem(w, http.StatusBadRequest, "invalid mock sequence name")
		return
	}
	writeControlJSON(w, resetResponse{Reset: s.ResetSequences(name)})
}

func (s *Server) controlClear(w http.ResponseWriter, r *http.Request) {
	var request struct{}
	if err := decodeControlRequest(w, r, &request); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	s.Clear()
	writeControlJSON(w, struct{}{})
}

func (s *Server) controlCount(w http.ResponseWriter, r *http.Request) {
	var pattern RequestPattern
	if err := decodeControlRequest(w, r, &pattern); err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	count, err := s.Count(r.Context(), pattern)
	if err != nil {
		status := http.StatusBadRequest
		var incomplete *IncompleteError
		if errors.As(err, &incomplete) {
			status = http.StatusConflict
		}
		writeProblem(w, status, err.Error())
		return
	}
	writeControlJSON(w, countResponse{Count: count})
}

func controlRequestAllowed(r *http.Request) bool {
	if r.Header.Get(controlHeader) != "1" || r.Header.Get("Origin") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func decodeControlRequest(w http.ResponseWriter, r *http.Request, dst any) error {
	defer func() { _ = r.Body.Close() }()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, controlBodyLimit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return fmt.Errorf("invalid mock control request: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("invalid mock control request: multiple JSON values")
		}
		return fmt.Errorf("invalid mock control request: %w", err)
	}
	return nil
}

func writeControlJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(value)
}
