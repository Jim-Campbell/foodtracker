package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/go-chi/chi/v5"
	chiMiddleware "github.com/go-chi/chi/v5/middleware"
)

type Config struct {
	APIKey string
	Photos bool
	AI     bool
	PWADir string
}

func NewRouter(cfg Config, h *Handler, log *slog.Logger) *chi.Mux {
	r := chi.NewRouter()
	r.Use(chiMiddleware.Recoverer)
	r.Use(chiMiddleware.RequestID)
	r.Use(chiMiddleware.RealIP)
	r.Use(LoggingMiddleware(log))

	r.Route("/api", func(r chi.Router) {
		// Health is unauthenticated.
		r.Get("/health", healthHandler(cfg.Photos, cfg.AI))

		// All other /api routes require auth. The wildcard catch-all ensures
		// the auth middleware runs even for unmatched paths (returning 404
		// only after a valid bearer token is presented).
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(cfg.APIKey))
			h.Routes(r)
			r.HandleFunc("/*", func(w http.ResponseWriter, r *http.Request) {
				writeError(w, http.StatusNotFound, "not found")
			})
		})
	})

	ServePWA(r, cfg.PWADir)
	return r
}

func healthHandler(photos, ai bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"ok":true,"photos":%t,"ai":%t}`, photos, ai)
	}
}

func ServePWA(r *chi.Mux, dir string) {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return
	}
	r.Get("/", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		http.ServeFile(w, req, filepath.Join(dir, "index.html"))
	})
	r.Get("/manifest.json", func(w http.ResponseWriter, req *http.Request) {
		http.ServeFile(w, req, filepath.Join(dir, "manifest.json"))
	})
	r.Get("/sw.js", func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/javascript")
		http.ServeFile(w, req, filepath.Join(dir, "sw.js"))
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
