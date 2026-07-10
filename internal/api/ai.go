package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jimgcampbell/food/internal/food"
)

// Parser is the subset of *ai.Parser the handler depends on. The progress
// callback (may be nil) receives user-facing status lines as the loop runs.
type Parser interface {
	ParseText(ctx context.Context, text, day string, progress func(string)) (*food.ParseResult, error)
}

// AIHandler serves the AI parse endpoints. It's nil-safe: when no
// ANTHROPIC_API_KEY is configured, parser is nil and every route returns 503.
type AIHandler struct {
	parser Parser
	log    *slog.Logger
}

func NewAIHandler(parser Parser, log *slog.Logger) *AIHandler {
	return &AIHandler{parser: parser, log: log}
}

func (h *AIHandler) Routes(r chi.Router) {
	r.Post("/parse", h.parse)
}

type parseRequest struct {
	Text string `json:"text"`
	Day  string `json:"day"`
}

func (h *AIHandler) parse(w http.ResponseWriter, r *http.Request) {
	if h.parser == nil {
		writeError(w, http.StatusServiceUnavailable, "AI parsing is not configured (set ANTHROPIC_API_KEY)")
		return
	}

	var req parseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Day); err != nil {
		writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
		return
	}

	// From here the response is an NDJSON progress stream (always 200);
	// failures travel as an error event.
	stream := newNDJSONStream(w)
	stream.Progress("Identifying foods…")
	result, err := h.parser.ParseText(r.Context(), req.Text, req.Day, stream.Progress)
	if err != nil {
		h.log.Error("parse meal failed", "error", err)
		stream.Error("failed to parse meal")
		return
	}
	stream.Result(result)
}
