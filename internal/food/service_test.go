package food

import (
	"context"
	"log/slog"
	"testing"
)

func newTestService() *Service {
	return NewService(newFakeStore(), slog.Default())
}

func TestServiceCreateMealRejectsBadDay(t *testing.T) {
	svc := newTestService()
	m := &Meal{Day: "not-a-date"}
	if err := svc.CreateMeal(context.Background(), m); err == nil {
		t.Error("expected an error for a malformed day, got nil")
	}
}

func TestServiceCreateMealRejectsBadItem(t *testing.T) {
	svc := newTestService()
	m := &Meal{
		Day: "2026-07-07",
		Items: []MealItem{
			{Name: "mystery meat", Tier: "not-a-tier", FractionPct: 100, Source: SourceManual},
		},
	}
	if err := svc.CreateMeal(context.Background(), m); err == nil {
		t.Error("expected an error for an invalid item tier, got nil")
	}
}

func TestServiceCreateMealDefaultsFractionAndInputKind(t *testing.T) {
	svc := newTestService()
	m := &Meal{
		Day:   "2026-07-07",
		Items: []MealItem{{Name: "apple", Calories: 95, Tier: TierHardYes}},
	}
	if err := svc.CreateMeal(context.Background(), m); err != nil {
		t.Fatalf("CreateMeal failed: %v", err)
	}
	if m.InputKind != InputManual {
		t.Errorf("InputKind = %q, want %q", m.InputKind, InputManual)
	}
	if m.Items[0].FractionPct != 100 {
		t.Errorf("FractionPct = %d, want 100", m.Items[0].FractionPct)
	}
}

func TestServiceDaySummaryRemaining(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Default settings: 1800 kcal, 165000 mg protein.
	m := &Meal{
		Day: "2026-07-07",
		Items: []MealItem{
			{Name: "big dinner", Calories: 2000, ProteinMg: 50000, Tier: TierHardYes, FractionPct: 100},
		},
	}
	if err := svc.CreateMeal(ctx, m); err != nil {
		t.Fatalf("CreateMeal failed: %v", err)
	}

	ds, err := svc.DaySummary(ctx, "2026-07-07")
	if err != nil {
		t.Fatalf("DaySummary failed: %v", err)
	}
	if ds.Calories != 2000 {
		t.Errorf("Calories = %d, want 2000", ds.Calories)
	}
	if ds.CaloriesRemaining != -200 {
		t.Errorf("CaloriesRemaining = %d, want -200 (over target)", ds.CaloriesRemaining)
	}
	if ds.ProteinRemainingMg != 115000 {
		t.Errorf("ProteinRemainingMg = %d, want 115000", ds.ProteinRemainingMg)
	}
	if ds.Score == nil || *ds.Score != 100 {
		t.Errorf("Score = %v, want 100", ds.Score)
	}
}

func TestServiceUpdateSettingsChangesRemaining(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	m := &Meal{
		Day:   "2026-07-07",
		Items: []MealItem{{Name: "snack", Calories: 500, Tier: TierNeutral, FractionPct: 100}},
	}
	if err := svc.CreateMeal(ctx, m); err != nil {
		t.Fatalf("CreateMeal failed: %v", err)
	}

	if _, err := svc.UpdateSettings(ctx, &Settings{CalorieTarget: 600, ProteinTargetMg: 100000}); err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}

	ds, err := svc.DaySummary(ctx, "2026-07-07")
	if err != nil {
		t.Fatalf("DaySummary failed: %v", err)
	}
	if ds.CaloriesRemaining != 100 {
		t.Errorf("CaloriesRemaining = %d, want 100 after target change", ds.CaloriesRemaining)
	}
}

func TestServiceUpdateSettingsRejectsNonPositiveTarget(t *testing.T) {
	svc := newTestService()
	if _, err := svc.UpdateSettings(context.Background(), &Settings{CalorieTarget: 0, ProteinTargetMg: 100000}); err == nil {
		t.Error("expected an error for a zero calorie_target, got nil")
	}
}

func TestServiceRangeSummaryOverTarget(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	over := &Meal{Day: "2026-07-01", Items: []MealItem{{Name: "feast", Calories: 2500, Tier: TierNeutral, FractionPct: 100}}}
	under := &Meal{Day: "2026-07-02", Items: []MealItem{{Name: "light day", Calories: 1200, Tier: TierNeutral, FractionPct: 100}}}
	if err := svc.CreateMeal(ctx, over); err != nil {
		t.Fatalf("CreateMeal failed: %v", err)
	}
	if err := svc.CreateMeal(ctx, under); err != nil {
		t.Fatalf("CreateMeal failed: %v", err)
	}

	rows, err := svc.RangeSummary(ctx, "2026-07-01", "2026-07-02")
	if err != nil {
		t.Fatalf("RangeSummary failed: %v", err)
	}
	byDay := map[string]RangeDay{}
	for _, r := range rows {
		byDay[r.Day] = r
	}
	if !byDay["2026-07-01"].OverTarget {
		t.Error("2026-07-01 should be over target (2500 > 1800)")
	}
	if byDay["2026-07-02"].OverTarget {
		t.Error("2026-07-02 should not be over target (1200 < 1800)")
	}
}
