package api

import (
	"encoding/json"
	"net/http"
	"sync"
)

// ndjsonStream writes newline-delimited JSON events over an open response,
// flushing after each, so the PWA can show parse progress while the AI loop
// runs. Events: {"type":"progress","message":...}, then exactly one
// {"type":"result","result":...} or {"type":"error","error":...}.
//
// The HTTP status is always 200 once streaming starts — errors after that
// point travel as an error event, so handlers must finish their request
// validation (4xx/503 via writeError) before calling newNDJSONStream.
type ndjsonStream struct {
	mu  sync.Mutex
	w   http.ResponseWriter
	rc  *http.ResponseController
	enc *json.Encoder
}

func newNDJSONStream(w http.ResponseWriter) *ndjsonStream {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	return &ndjsonStream{w: w, rc: http.NewResponseController(w), enc: json.NewEncoder(w)}
}

func (s *ndjsonStream) send(v any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Encode appends the newline; a failed write means the client went away,
	// which the parse loop will notice via the request context.
	_ = s.enc.Encode(v)
	_ = s.rc.Flush()
}

func (s *ndjsonStream) Progress(msg string) {
	s.send(map[string]string{"type": "progress", "message": msg})
}

func (s *ndjsonStream) Result(v any) {
	s.send(map[string]any{"type": "result", "result": v})
}

func (s *ndjsonStream) Error(msg string) {
	s.send(map[string]string{"type": "error", "error": msg})
}
