package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jimgcampbell/food/internal/food"
)

// maxPhotoBytes is the upload size cap, enforced both via MaxBytesReader and
// the multipart form's memory limit.
const maxPhotoBytes = 10 << 20 // 10 MB

var photoExtByContentType = map[string]string{
	"image/jpeg": "jpg",
	"image/png":  "png",
	"image/webp": "webp",
	"image/heic": "heic",
}

// PhotoStore is the subset of *storage.R2Client the handler depends on.
type PhotoStore interface {
	UploadPhoto(ctx context.Context, key, contentType string, data io.Reader) (string, error)
	GetPhoto(ctx context.Context, key string) ([]byte, string, error)
}

// ImageParser is the subset of *ai.Parser used for vision parses. The
// progress callback (may be nil) receives user-facing status lines.
type ImageParser interface {
	ParseImage(ctx context.Context, imageData []byte, mediaType, hint, day string, progress func(string)) (*food.ParseResult, error)
}

// PhotoHandler serves photo upload and vision-parse endpoints. Nil-safe:
// store is nil when R2 isn't configured (both routes 503); analyze also
// needs parser (AI), so it 503s if either is unconfigured.
type PhotoHandler struct {
	store  PhotoStore
	parser ImageParser
	log    *slog.Logger
}

func NewPhotoHandler(store PhotoStore, parser ImageParser, log *slog.Logger) *PhotoHandler {
	return &PhotoHandler{store: store, parser: parser, log: log}
}

func (h *PhotoHandler) Routes(r chi.Router) {
	r.Post("/photos", h.upload)
	r.Post("/analyze-photo", h.analyze)
}

func (h *PhotoHandler) upload(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "photos are not configured (set R2_* vars)")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoBytes+1<<20) // headroom for multipart overhead
	if err := r.ParseMultipartForm(maxPhotoBytes); err != nil {
		writeError(w, http.StatusBadRequest, "invalid multipart form or image over the 10MB limit")
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		writeError(w, http.StatusBadRequest, "missing image field")
		return
	}
	defer file.Close()

	if header.Size > maxPhotoBytes {
		writeError(w, http.StatusBadRequest, "image exceeds the 10MB limit")
		return
	}

	contentType := header.Header.Get("Content-Type")
	ext, ok := photoExtByContentType[contentType]
	if !ok {
		writeError(w, http.StatusBadRequest, "unsupported content type "+contentType+" (want image/jpeg, image/png, image/webp, or image/heic)")
		return
	}

	key := objectKey(ext)
	url, err := h.store.UploadPhoto(r.Context(), key, contentType, file)
	if err != nil {
		h.log.Error("upload photo failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to upload photo")
		return
	}
	// The url is what the PWA's <img> tags will request -- logged so a broken
	// thumbnail can be diagnosed by opening this exact URL.
	h.log.Info("photo uploaded", "key", key, "url", url, "bytes", header.Size, "content_type", contentType)
	writeJSON(w, http.StatusCreated, map[string]string{"key": key, "url": url})
}

// objectKey builds a food/YYYY/MM/<random>.<ext> R2 key.
func objectKey(ext string) string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("generate photo key: %v", err)) // unreachable: crypto/rand.Read only fails if the OS RNG is broken
	}
	return fmt.Sprintf("food/%s/%s.%s", time.Now().UTC().Format("2006/01"), hex.EncodeToString(b), ext)
}

type analyzePhotoRequest struct {
	Key  string `json:"key"`
	Hint string `json:"hint"`
	Day  string `json:"day"`
}

func (h *PhotoHandler) analyze(w http.ResponseWriter, r *http.Request) {
	if h.store == nil || h.parser == nil {
		writeError(w, http.StatusServiceUnavailable, "photo analysis is not configured (set R2_* vars and ANTHROPIC_API_KEY)")
		return
	}

	var req analyzePhotoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	if _, err := time.Parse("2006-01-02", req.Day); err != nil {
		writeError(w, http.StatusBadRequest, "day must be YYYY-MM-DD")
		return
	}

	data, contentType, err := h.store.GetPhoto(r.Context(), req.Key)
	if err != nil {
		h.log.Error("fetch photo failed", "key", req.Key, "error", err)
		writeError(w, http.StatusInternalServerError, "failed to fetch photo")
		return
	}
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// From here the response is an NDJSON progress stream (always 200);
	// failures travel as an error event.
	stream := newNDJSONStream(w)
	stream.Progress("Reading your photo…")
	result, err := h.parser.ParseImage(r.Context(), data, contentType, req.Hint, req.Day, stream.Progress)
	if err != nil {
		h.log.Error("analyze photo failed", "error", err)
		stream.Error("failed to analyze photo")
		return
	}
	stream.Result(result)
}
