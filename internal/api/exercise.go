package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jimgcampbell/food/internal/food"
)

// exerciseRoutes wires the exercise-session endpoints (phase E1). Exercise is
// fully calorie-independent — nothing here touches the meals/day-summary code.
func (h *Handler) exerciseRoutes(r chi.Router) {
	r.Post("/exercise", h.createExercise)
	r.Get("/exercise", h.listExercise)
	r.Get("/exercise/{id}", h.getExercise)
	r.Put("/exercise/{id}", h.updateExercise)
	r.Delete("/exercise/{id}", h.deleteExercise)
}

func (h *Handler) createExercise(w http.ResponseWriter, r *http.Request) {
	var e food.ExerciseSession
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	if err := h.svc.CreateExercise(r.Context(), &e); err != nil {
		h.fail(w, "create exercise session", err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (h *Handler) listExercise(w http.ResponseWriter, r *http.Request) {
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		writeError(w, http.StatusBadRequest, "start and end are required")
		return
	}
	sessions, err := h.svc.ListExerciseRange(r.Context(), start, end)
	if err != nil {
		h.fail(w, "list exercise sessions", err)
		return
	}
	writeJSON(w, http.StatusOK, sessions)
}

func (h *Handler) getExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseExerciseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := h.svc.GetExercise(r.Context(), id)
	if err != nil {
		h.fail(w, "get exercise session", err)
		return
	}
	writeJSON(w, http.StatusOK, e)
}

func (h *Handler) updateExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseExerciseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var e food.ExerciseSession
	if err := json.NewDecoder(r.Body).Decode(&e); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}
	e.ID = id
	if err := h.svc.UpdateExercise(r.Context(), &e); err != nil {
		h.fail(w, "update exercise session", err)
		return
	}
	updated, err := h.svc.GetExercise(r.Context(), id)
	if err != nil {
		h.fail(w, "get exercise session", err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *Handler) deleteExercise(w http.ResponseWriter, r *http.Request) {
	id, err := parseExerciseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.svc.DeleteExercise(r.Context(), id); err != nil {
		h.fail(w, "delete exercise session", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func parseExerciseID(r *http.Request) (int64, error) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid exercise session id")
	}
	return id, nil
}
