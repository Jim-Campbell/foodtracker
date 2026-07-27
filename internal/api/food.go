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
	// A food is the first-class logged unit; a meal is the (day, slot)
	// container. Logging appends foods; the container is read/cleared as a unit.
	r.Post("/foods", h.addFoods)
	r.Get("/foods/{id}", h.getFood)
	r.Put("/foods/{id}", h.updateFood)
	r.Delete("/foods/{id}", h.deleteFood)

	r.Get("/meals", h.listMeals)
	r.Get("/meals/{id}", h.getMeal)
	r.Delete("/meals/{id}", h.deleteMeal) // clears the whole slot

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
	r.Get("/export/range", h.exportRange)
	r.Get("/export/analysis", h.exportAnalysis)

	// Inclusion score (inclusion phase 1): independent of the day/range
	// composition score above, never merged with it.
	r.Get("/components", h.listComponents)
	r.Get("/inclusion", h.inclusionWindow)
	r.Get("/inclusion/weeks", h.inclusionWeeks)
	r.Get("/component-tags", h.listComponentTags)
	r.Put("/component-tags", h.setComponentTags)
	r.Delete("/component-tags/{name}", h.deleteComponentTags)
	r.Get("/component-tags/candidates", h.componentTagCandidates)

	h.exerciseRoutes(r)
}

// ---- foods (first-class items) ----

type addFoodsRequest struct {
	Day   string          `json:"day"`
	Slot  *string         `json:"slot"`
	Items []food.MealItem `json:"items"`
}

func (h *Handler) addFoods(w http.ResponseWriter, r *http.Request) {
	var req addFoodsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	meal, err := h.svc.AddFoods(r.Context(), req.Day, req.Slot, req.Items)
	if err != nil {
		h.fail(w, "add foods", err)
		return
	}
	writeJSON(w, http.StatusCreated, meal)
}

// updateFoodRequest embeds the food fields flat alongside the target day/slot,
// so editing a food and moving its slot is one request.
type updateFoodRequest struct {
	Day  string  `json:"day"`
	Slot *string `json:"slot"`
	food.MealItem
}

func (h *Handler) getFood(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	it, err := h.svc.GetFood(r.Context(), id)
	if err != nil {
		h.fail(w, "get food", err)
		return
	}
	writeJSON(w, http.StatusOK, it)
}

func (h *Handler) updateFood(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var req updateFoodRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	req.MealItem.ID = id
	if err := h.svc.UpdateFood(r.Context(), req.Day, req.Slot, &req.MealItem); err != nil {
		h.fail(w, "update food", err)
		return
	}
	updated, err := h.svc.GetFood(r.Context(), id)
	if err != nil {
		h.fail(w, "get food", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteFood(w http.ResponseWriter, r *http.Request) {
	id, err := parseMealID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.DeleteFood(r.Context(), id); err != nil {
		h.fail(w, "delete food", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- meals (containers) ----

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

// exportRange reports the earliest and latest logged day so the PWA can
// default the analysis-export date pickers to the full range. Returns {} when
// there is no data yet.
func (h *Handler) exportRange(w http.ResponseWriter, r *http.Request) {
	start, end, ok, err := h.svc.DataRange(r.Context())
	if err != nil {
		h.fail(w, "export range", err)
		return
	}
	if !ok {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"start": start, "end": end})
}

// exportAnalysis returns the reshaped, LLM-ready export for a day range.
// Missing start/end default to the full logged range (today if the DB is
// empty), so the endpoint is useful with no query params at all.
func (h *Handler) exportAnalysis(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		ds, de, ok, err := h.svc.DataRange(r.Context())
		if err != nil {
			h.fail(w, "export range", err)
			return
		}
		if !ok {
			today := time.Now().UTC().Format("2006-01-02")
			ds, de = today, today
		}
		if start == "" {
			start = ds
		}
		if end == "" {
			end = de
		}
	}
	doc, err := h.svc.ExportAnalysis(r.Context(), start, end)
	if err != nil {
		h.fail(w, "export analysis", err)
		return
	}
	filename := fmt.Sprintf("food-analysis-%s_%s.json", start, end)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	writeJSON(w, http.StatusOK, doc)
}

// ---- inclusion (component tags + rolling/weekly score) ----

func (h *Handler) listComponents(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, food.ComponentCatalog)
}

func (h *Handler) inclusionWindow(w http.ResponseWriter, r *http.Request) {
	end := r.URL.Query().Get("end")
	win, err := h.svc.InclusionWindow(r.Context(), end)
	if err != nil {
		h.fail(w, "inclusion window", err)
		return
	}
	writeJSON(w, http.StatusOK, win)
}

func (h *Handler) inclusionWeeks(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start and end are required")
		return
	}
	weeks, err := h.svc.InclusionWeeks(r.Context(), start, end)
	if err != nil {
		h.fail(w, "inclusion weeks", err)
		return
	}
	if weeks == nil {
		weeks = []food.InclusionWeek{}
	}
	writeJSON(w, http.StatusOK, weeks)
}

func (h *Handler) listComponentTags(w http.ResponseWriter, r *http.Request) {
	tags, err := h.svc.ListComponentTags(r.Context())
	if err != nil {
		h.fail(w, "list component tags", err)
		return
	}
	writeJSON(w, http.StatusOK, tags)
}

type setComponentTagsRequest struct {
	NormalizedName string              `json:"normalized_name"`
	FDCID          *int64              `json:"fdc_id"`
	Tags           []food.ComponentTag `json:"tags"`
}

func (h *Handler) setComponentTags(w http.ResponseWriter, r *http.Request) {
	var req setComponentTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.SetComponentTags(r.Context(), req.NormalizedName, req.FDCID, req.Tags); err != nil {
		h.fail(w, "set component tags", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// componentTagCandidates serves the Settings → "Tag foods" backfill screen
// (inclusion phase 2 §4): distinct logged foods, untagged first.
func (h *Handler) componentTagCandidates(w http.ResponseWriter, r *http.Request) {
	candidates, err := h.svc.TagCandidates(r.Context())
	if err != nil {
		h.fail(w, "list tag candidates", err)
		return
	}
	if candidates == nil {
		candidates = []food.TagCandidate{}
	}
	writeJSON(w, http.StatusOK, candidates)
}

func (h *Handler) deleteComponentTags(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if err := h.svc.DeleteComponentTags(r.Context(), name); err != nil {
		h.fail(w, "delete component tags", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
