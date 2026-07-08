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
	if err := s.store.UpdateSettings(ctx, in); err != nil {
		return nil, err
	}
	return s.store.GetSettings(ctx)
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
	return &ExportDoc{
		ExportedAt: time.Now().UTC(),
		Settings:   *settings,
		Weights:    weights,
		Meals:      meals,
	}, nil
}
