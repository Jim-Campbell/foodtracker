package food

import (
	"context"
	"testing"
)

// TestExportAnalysis checks the reshaped export: as-eaten values (fraction
// applied), mg->g and g->lb conversion, the per-day rollup, per-day exercise
// summary, and date filtering.
func TestExportAnalysis(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// A dinner with a half-portion item: full-portion 400 kcal / 40 g protein,
	// eaten at 50% => 200 kcal / 20 g. Grams 300 => 150 as-eaten.
	m := &Meal{
		Day:  "2026-07-10",
		Slot: strPtr(SlotDinner),
		Items: []MealItem{{
			Name: "salmon", Grams: intPtr(300), FractionPct: 50,
			Calories: 400, ProteinMg: 40000, CarbsMg: 0, FatMg: 20000, SodiumMg: 200,
			Tier: TierHardYes, Source: SourceManual,
		}},
	}
	if err := svc.CreateMeal(ctx, m); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}
	if err := svc.UpsertWeight(ctx, &Weight{Day: "2026-07-10", WeightG: 85004}); err != nil {
		t.Fatalf("UpsertWeight: %v", err)
	}
	if err := svc.CreateExercise(ctx, &ExerciseSession{Day: "2026-07-10", Type: ExerciseCardio, Activity: strPtr("Run"), DurationMin: intPtr(45)}); err != nil {
		t.Fatalf("CreateExercise cardio: %v", err)
	}
	if err := svc.CreateExercise(ctx, &ExerciseSession{Day: "2026-07-10", Type: ExerciseStrength, Note: "gym"}); err != nil {
		t.Fatalf("CreateExercise strength: %v", err)
	}
	// Out-of-range meal must not appear.
	if err := svc.CreateMeal(ctx, &Meal{Day: "2026-08-01", Items: []MealItem{{Name: "toast", Calories: 100, Tier: TierNeutral, FractionPct: 100}}}); err != nil {
		t.Fatalf("CreateMeal out of range: %v", err)
	}

	doc, err := svc.ExportAnalysis(ctx, "2026-07-01", "2026-07-31")
	if err != nil {
		t.Fatalf("ExportAnalysis: %v", err)
	}
	if len(doc.Days) != 1 {
		t.Fatalf("Days = %d, want 1 (out-of-range day excluded)", len(doc.Days))
	}
	d := doc.Days[0]
	if d.Date != "2026-07-10" {
		t.Errorf("Date = %q", d.Date)
	}
	if d.Calories != 200 {
		t.Errorf("Calories = %d, want 200 (as-eaten)", d.Calories)
	}
	if d.ProteinG != 20 {
		t.Errorf("ProteinG = %v, want 20", d.ProteinG)
	}
	if d.QualityScore == nil || *d.QualityScore != 100 {
		t.Errorf("QualityScore = %v, want 100", d.QualityScore)
	}
	if d.WeightLb == nil || *d.WeightLb != 187.4 {
		t.Errorf("WeightLb = %v, want 187.4", d.WeightLb)
	}
	if d.Exercise.CardioMin != 45 || d.Exercise.StrengthSessions != 1 || d.Exercise.ActiveMin != 45 {
		t.Errorf("Exercise summary = %+v", d.Exercise)
	}

	if len(doc.Meals) != 1 {
		t.Fatalf("Meals = %d, want 1", len(doc.Meals))
	}
	item := doc.Meals[0].Items[0]
	if item.Grams == nil || *item.Grams != 150 {
		t.Errorf("item Grams = %v, want 150 (as-eaten)", item.Grams)
	}
	if item.Calories != 200 || item.ProteinG != 20 {
		t.Errorf("item as-eaten = %d kcal / %v g protein", item.Calories, item.ProteinG)
	}
	if len(doc.Exercise) != 2 {
		t.Errorf("Exercise sessions = %d, want 2", len(doc.Exercise))
	}
	if len(doc.Weights) != 1 || doc.Weights[0].WeightLb != 187.4 {
		t.Errorf("Weights = %+v", doc.Weights)
	}
}

func TestExportAnalysisRejectsInvertedRange(t *testing.T) {
	svc := newTestService()
	if _, err := svc.ExportAnalysis(context.Background(), "2026-07-31", "2026-07-01"); err == nil {
		t.Error("expected an error when start is after end, got nil")
	}
}

func TestDataRange(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	if _, _, ok, err := svc.DataRange(ctx); err != nil || ok {
		t.Fatalf("empty DataRange: ok=%v err=%v, want ok=false", ok, err)
	}

	if err := svc.CreateMeal(ctx, &Meal{Day: "2026-07-10", Items: []MealItem{{Name: "egg", Calories: 70, Tier: TierNeutral, FractionPct: 100}}}); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}
	if err := svc.CreateExercise(ctx, &ExerciseSession{Day: "2026-07-02", Type: ExerciseStrength}); err != nil {
		t.Fatalf("CreateExercise: %v", err)
	}
	if err := svc.UpsertWeight(ctx, &Weight{Day: "2026-07-20", WeightG: 85000}); err != nil {
		t.Fatalf("UpsertWeight: %v", err)
	}

	start, end, ok, err := svc.DataRange(ctx)
	if err != nil || !ok {
		t.Fatalf("DataRange: ok=%v err=%v", ok, err)
	}
	if start != "2026-07-02" || end != "2026-07-20" {
		t.Errorf("DataRange = %s..%s, want 2026-07-02..2026-07-20", start, end)
	}
}
