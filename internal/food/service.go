package food

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

type Service struct {
	store Store
	log   *slog.Logger
}

func NewService(store Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

func validDate(s string) error {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("invalid: date must be YYYY-MM-DD")
	}
	return nil
}

// validateMeal checks meal-level fields and every item, filling in defaults
// (input_kind, fraction_pct) the way a manual entry would expect. Item
// warnings (Atwater mismatches) clamp confidence to low but don't block the
// save; item errors (bad enum, negative amount) do.
func validateMeal(m *Meal) error {
	if strings.TrimSpace(m.Day) == "" {
		return fmt.Errorf("invalid: day is required")
	}
	if err := validDate(m.Day); err != nil {
		return err
	}
	if m.Slot != nil && !validSlots[*m.Slot] {
		return fmt.Errorf("invalid: bad slot")
	}
	if m.InputKind == "" {
		m.InputKind = InputManual
	}
	if !validInputKinds[m.InputKind] {
		return fmt.Errorf("invalid: bad input_kind")
	}
	for i := range m.Items {
		it := &m.Items[i]
		if it.FractionPct == 0 {
			it.FractionPct = 100
		}
		if it.Tier == "" {
			it.Tier = TierNeutral
		}
		if it.Source == "" {
			it.Source = SourceManual
		}
		if it.Confidence == "" {
			it.Confidence = ConfidenceMedium
		}
		errs, warnings := ValidateItem(*it)
		if len(errs) > 0 {
			return fmt.Errorf("invalid: item %d (%s): %s", i, it.Name, strings.Join(errs, "; "))
		}
		if len(warnings) > 0 {
			it.Confidence = ConfidenceLow
		}
	}
	return nil
}

func (s *Service) CreateMeal(ctx context.Context, m *Meal) error {
	if err := validateMeal(m); err != nil {
		return err
	}
	return s.store.CreateMeal(ctx, m)
}

func (s *Service) GetMeal(ctx context.Context, id int64) (*Meal, error) {
	m, err := s.store.GetMeal(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("not found: meal")
	}
	return m, nil
}

func (s *Service) ListMealsByDay(ctx context.Context, day string) ([]Meal, error) {
	if err := validDate(day); err != nil {
		return nil, err
	}
	return s.store.ListMealsByDay(ctx, day)
}

func (s *Service) UpdateMeal(ctx context.Context, m *Meal) error {
	existing, err := s.store.GetMeal(ctx, m.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: meal")
	}
	if err := validateMeal(m); err != nil {
		return err
	}
	return s.store.UpdateMeal(ctx, m)
}

func (s *Service) DeleteMeal(ctx context.Context, id int64) error {
	existing, err := s.store.GetMeal(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: meal")
	}
	return s.store.DeleteMeal(ctx, id)
}

// ---- weights ----

func (s *Service) UpsertWeight(ctx context.Context, w *Weight) error {
	if err := validDate(w.Day); err != nil {
		return err
	}
	if w.WeightG <= 0 {
		return fmt.Errorf("invalid: weight_g must be positive")
	}
	return s.store.UpsertWeight(ctx, w)
}

func (s *Service) ListWeights(ctx context.Context, start, end string) ([]Weight, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	return s.store.ListWeights(ctx, start, end)
}

func (s *Service) DeleteWeight(ctx context.Context, day string) error {
	if err := validDate(day); err != nil {
		return err
	}
	return s.store.DeleteWeight(ctx, day)
}

// ---- favorites ----

func (s *Service) CreateFavorite(ctx context.Context, f *Favorite) error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return fmt.Errorf("invalid: favorite name is required")
	}
	if len(f.Items) == 0 {
		return fmt.Errorf("invalid: favorite needs at least one item")
	}
	for i := range f.Items {
		// A favorite is a template, not a row reference.
		f.Items[i].ID = 0
		f.Items[i].MealID = 0
		if errs, _ := ValidateItem(f.Items[i]); len(errs) > 0 {
			return fmt.Errorf("invalid: item %q: %s", f.Items[i].Name, strings.Join(errs, "; "))
		}
	}
	return s.store.CreateFavorite(ctx, f)
}

func (s *Service) ListFavorites(ctx context.Context) ([]Favorite, error) {
	favs, err := s.store.ListFavorites(ctx)
	if err != nil {
		return nil, err
	}
	if favs == nil {
		favs = []Favorite{}
	}
	return favs, nil
}

func (s *Service) RenameFavorite(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("invalid: favorite name is required")
	}
	// The store reports a missing row / duplicate name itself.
	return s.store.RenameFavorite(ctx, id, name)
}

func (s *Service) DeleteFavorite(ctx context.Context, id int64) error {
	// The store reports a missing row as a "not found:" error itself.
	return s.store.DeleteFavorite(ctx, id)
}

// ---- settings ----

func (s *Service) GetSettings(ctx context.Context) (*Settings, error) {
	return s.store.GetSettings(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, in *Settings) (*Settings, error) {
	if in.CalorieTarget <= 0 {
		return nil, fmt.Errorf("invalid: calorie_target must be positive")
	}
	if in.ProteinTargetMg <= 0 {
		return nil, fmt.Errorf("invalid: protein_target_mg must be positive")
	}
	if in.WeightTargetG != nil && *in.WeightTargetG <= 0 {
		return nil, fmt.Errorf("invalid: weight_target_g must be positive")
	}
	if in.CardioWeeklyTarget < 0 || in.StrengthWeeklyTarget < 0 || in.YogaWeeklyTarget < 0 || in.MeditationWeeklyDays < 0 || in.PTWeeklyDays < 0 {
		return nil, fmt.Errorf("invalid: weekly targets must be non-negative")
	}
	if err := s.store.UpdateSettings(ctx, in); err != nil {
		return nil, err
	}
	return s.store.GetSettings(ctx)
}

// ---- exercise ----

// ExerciseFieldErrors checks the per-type field requirements settled in the
// design spike: cardio needs activity+duration, yoga needs
// location+style+duration, meditation needs duration; strength and pt carry no
// required fields beyond type (their note holds any detail) and must NOT have a
// duration. Exported so the AI parser (phase E4 natural-language logging) can
// flag an incomplete parsed session in the draft without duplicating these
// rules.
func ExerciseFieldErrors(e ExerciseSession) (errs []string) {
	switch e.Type {
	case ExerciseCardio:
		if e.Activity == nil || strings.TrimSpace(*e.Activity) == "" {
			errs = append(errs, "cardio requires activity")
		}
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "cardio requires duration_min > 0")
		}
	case ExerciseYoga:
		if e.Location == nil || strings.TrimSpace(*e.Location) == "" {
			errs = append(errs, "yoga requires location")
		}
		if e.Style == nil || strings.TrimSpace(*e.Style) == "" {
			errs = append(errs, "yoga requires style")
		}
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "yoga requires duration_min > 0")
		}
	case ExerciseMeditation:
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "meditation requires duration_min > 0")
		}
	case ExercisePT:
		if e.DurationMin != nil {
			errs = append(errs, "pt must not have duration_min")
		}
	case ExerciseStrength:
		if e.DurationMin != nil {
			errs = append(errs, "strength must not have duration_min")
		}
	}
	// HR zones are cardio-only; keys must be "1".."5" and minutes non-negative.
	if len(e.HRZones) > 0 {
		if e.Type != ExerciseCardio {
			errs = append(errs, "hr_zones only allowed on cardio")
		}
		for z, min := range e.HRZones {
			if len(z) != 1 || z < "1" || z > "5" {
				errs = append(errs, "hr_zones has invalid zone: "+z)
			}
			if min < 0 {
				errs = append(errs, "hr_zones minutes must be non-negative")
			}
		}
	}
	return errs
}

// validateExercise checks day/type/input_kind enums and the per-type field
// requirements (see ExerciseFieldErrors).
func validateExercise(e *ExerciseSession) error {
	if err := validDate(e.Day); err != nil {
		return err
	}
	if !validExerciseTypes[e.Type] {
		return fmt.Errorf("invalid: bad exercise type")
	}
	if e.InputKind == "" {
		e.InputKind = ExerciseInputTap
	}
	if !validExerciseInputKinds[e.InputKind] {
		return fmt.Errorf("invalid: bad input_kind")
	}
	if e.DurationMin != nil && *e.DurationMin < 0 {
		return fmt.Errorf("invalid: duration_min must be non-negative")
	}
	if errs := ExerciseFieldErrors(*e); len(errs) > 0 {
		return fmt.Errorf("invalid: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) CreateExercise(ctx context.Context, e *ExerciseSession) error {
	if err := validateExercise(e); err != nil {
		return err
	}
	return s.store.CreateExercise(ctx, e)
}

func (s *Service) GetExercise(ctx context.Context, id int64) (*ExerciseSession, error) {
	e, err := s.store.GetExercise(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("not found: exercise session")
	}
	return e, nil
}

func (s *Service) UpdateExercise(ctx context.Context, e *ExerciseSession) error {
	existing, err := s.store.GetExercise(ctx, e.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: exercise session")
	}
	if err := validateExercise(e); err != nil {
		return err
	}
	return s.store.UpdateExercise(ctx, e)
}

func (s *Service) DeleteExercise(ctx context.Context, id int64) error {
	existing, err := s.store.GetExercise(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: exercise session")
	}
	return s.store.DeleteExercise(ctx, id)
}

func (s *Service) ListExerciseRange(ctx context.Context, start, end string) ([]ExerciseSession, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	sessions, err := s.store.ListExerciseRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if sessions == nil {
		sessions = []ExerciseSession{}
	}
	return sessions, nil
}

// ---- summaries ----

func (s *Service) DaySummary(ctx context.Context, day string) (*DaySummary, error) {
	if err := validDate(day); err != nil {
		return nil, err
	}
	ds, err := s.store.DaySummary(ctx, day)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	ds.CalorieTarget = settings.CalorieTarget
	ds.ProteinTargetMg = settings.ProteinTargetMg
	ds.CaloriesRemaining = int64(settings.CalorieTarget) - ds.Calories
	ds.ProteinRemainingMg = settings.ProteinTargetMg - ds.ProteinMg
	return ds, nil
}

func (s *Service) RangeSummary(ctx context.Context, start, end string) ([]RangeDay, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	rows, err := s.store.RangeSummary(ctx, start, end)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].OverTarget = rows[i].Calories > int64(settings.CalorieTarget)
	}
	return rows, nil
}

// ---- export ----

func (s *Service) Export(ctx context.Context) (*ExportDoc, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	weights, err := s.store.ListAllWeights(ctx)
	if err != nil {
		return nil, err
	}
	if weights == nil {
		weights = []Weight{}
	}
	meals, err := s.store.ListAllMeals(ctx)
	if err != nil {
		return nil, err
	}
	if meals == nil {
		meals = []Meal{}
	}
	favorites, err := s.ListFavorites(ctx)
	if err != nil {
		return nil, err
	}
	exercise, err := s.store.ListAllExercise(ctx)
	if err != nil {
		return nil, err
	}
	if exercise == nil {
		exercise = []ExerciseSession{}
	}
	return &ExportDoc{
		ExportedAt: time.Now().UTC(),
		Settings:   *settings,
		Weights:    weights,
		Meals:      meals,
		Favorites:  favorites,
		Exercise:   exercise,
	}, nil
}
