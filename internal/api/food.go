package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jimgcampbell/food/internal/food"
)

// Handler serves the meals/weights/settings/summary API (phase 2). AI and
// photo endpoints are wired in by later phases.
type Handler struct {
	svc *food.Service
	log *slog.Logger
}

func NewHandler(svc *food.Service, log *slog.Logger) *Handler {
	return &Handler{svc: svc, log: log}
}

func (h *Handler) Routes(r chi.Router) {
	r.Post("/meals", h.createMeal)
	r.Get("/meals", h.listMeals)
	r.Get("/meals/{id}", h.getMeal)
	r.Put("/meals/{id}", h.updateMeal)
	r.Delete("/meals/{id}", h.deleteMeal)

	r.Get("/day/{date}", h.daySummary)
	r.Get("/range", h.rangeSummary)

	r.Post("/weights", h.upsertWeight)
	r.Get("/weights", h.listWeights)
	r.Delete("/weights/{day}", h.deleteWeight)

	r.Post("/favorites", h.createFavorite)
	r.Get("/favorites", h.listFavorites)
	r.Put("/favorites/{id}", h.renameFavorite)
	r.Delete("/favorites/{id}", h.deleteFavorite)

	r.Get("/settings", h.getSettings)
	r.Put("/settings", h.updateSettings)

	r.Get("/export", h.export)

	h.exerciseRoutes(r)
}

// ---- meals ----

func (h *Handler) createMeal(w http.ResponseWriter, r *http.Request) {
	var m food.Meal
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.CreateMeal(r.Context(), &m); err != nil {
		h.fail(w, "create meal", err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

func (h *Handler) listMeals(w http.ResponseWriter, r *http.Request) {
	day := r.URL.Query().Get("day")
	if day == "" {
		writeError(w, http.StatusBadRequest, "day is required")
		return
	}
	meals, err := h.svc.ListMealsByDay(r.Context(), day)
	if err != nil {
		h.fail(w, "list meals", err)
		return
	}
	if meals == nil {
		meals = []food.Meal{}
	}
	writeJSON(w, http.StatusOK, meals)
}

func (h *Handler) getMeal(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	meal, err := h.svc.GetMeal(r.Context(), id)
	if err != nil {
		h.fail(w, "get meal", err)
		return
	}
	writeJSON(w, http.StatusOK, meal)
}

func (h *Handler) updateMeal(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var m food.Meal
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	m.ID = id
	if err := h.svc.UpdateMeal(r.Context(), &m); err != nil {
		h.fail(w, "update meal", err)
		return
	}
	updated, err := h.svc.GetMeal(r.Context(), id)
	if err != nil {
		h.fail(w, "get meal", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteMeal(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.DeleteMeal(r.Context(), id); err != nil {
		h.fail(w, "delete meal", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseMealID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid meal id")
	}
	return id, nil
}

// ---- favorites ----

func (h *Handler) createFavorite(w http.ResponseWriter, r *http.Request) {
	var f food.Favorite
	if err := json.NewDecoder(r.Body).Decode(&f); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.CreateFavorite(r.Context(), &f); err != nil {
		h.fail(w, "create favorite", err)
		return
	}
	writeJSON(w, http.StatusCreated, f)
}

func (h *Handler) listFavorites(w http.ResponseWriter, r *http.Request) {
	favs, err := h.svc.ListFavorites(r.Context())
	if err != nil {
		h.fail(w, "list favorites", err)
		return
	}
	writeJSON(w, http.StatusOK, favs)
}

func (h *Handler) renameFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid favorite id")
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.RenameFavorite(r.Context(), id, body.Name); err != nil {
		h.fail(w, "rename favorite", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) deleteFavorite(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid favorite id")
		return
	}
	if err := h.svc.DeleteFavorite(r.Context(), id); err != nil {
		h.fail(w, "delete favorite", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- summaries ----

func (h *Handler) daySummary(w http.ResponseWriter, r *http.Request) {
	ds, err := h.svc.DaySummary(r.Context(), chi.URLParam(r, "date"))
	if err != nil {
		h.fail(w, "day summary", err)
		return
	}
	writeJSON(w, http.StatusOK, ds)
}

func (h *Handler) rangeSummary(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start and end are required")
		return
	}
	rows, err := h.svc.RangeSummary(r.Context(), start, end)
	if err != nil {
		h.fail(w, "range summary", err)
		return
	}
	if rows == nil {
		rows = []food.RangeDay{}
	}
	writeJSON(w, http.StatusOK, rows)
}

// ---- weights ----

func (h *Handler) upsertWeight(w http.ResponseWriter, r *http.Request) {
	var wt food.Weight
	if err := json.NewDecoder(r.Body).Decode(&wt); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.UpsertWeight(r.Context(), &wt); err != nil {
		h.fail(w, "upsert weight", err)
		return
	}
	writeJSON(w, http.StatusOK, wt)
}

func (h *Handler) listWeights(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start and end are required")
		return
	}
	weights, err := h.svc.ListWeights(r.Context(), start, end)
	if err != nil {
		h.fail(w, "list weights", err)
		return
	}
	if weights == nil {
		weights = []food.Weight{}
	}
	writeJSON(w, http.StatusOK, weights)
}

func (h *Handler) deleteWeight(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.DeleteWeight(r.Context(), chi.URLParam(r, "day")); err != nil {
		h.fail(w, "delete weight", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- settings ----

func (h *Handler) getSettings(w http.ResponseWriter, r *http.Request) {
	s, err := h.svc.GetSettings(r.Context())
	if err != nil {
		h.fail(w, "get settings", err)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

func (h *Handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var s food.Settings
	if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	updated, err := h.svc.UpdateSettings(r.Context(), &s)
	if err != nil {
		h.fail(w, "update settings", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// ---- export ----

// export streams the full DB as a single JSON document — the backup story
// for Render's ephemeral disk (see internal/backup in the sibling finance
// app for why this matters). Dependency-free: encoding/json only.
func (h *Handler) export(w http.ResponseWriter, r *http.Request) {
	doc, err := h.svc.Export(r.Context())
	if err != nil {
		h.fail(w, "export", err)
		return
	}
	filename := "food-export-" + time.Now().UTC().Format("20060102") + ".json"
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	writeJSON(w, http.StatusOK, doc)
}

// ---- errors ----

// fail maps service errors to status codes via sentinel prefixes
// ("invalid:", "not found:"), same convention as journal/finance.
func (h *Handler) fail(w http.ResponseWriter, op string, err error) {
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "invalid:"):
		writeError(w, http.StatusBadRequest, strings.TrimSpace(strings.TrimPrefix(msg, "invalid:")))
	case strings.HasPrefix(msg, "not found:"):
		writeError(w, http.StatusNotFound, strings.TrimSpace(strings.TrimPrefix(msg, "not found:")))
	default:
		h.log.Error(op+" failed", "error", err)
		writeError(w, http.StatusInternalServerError, "failed to "+op)
	}
}
