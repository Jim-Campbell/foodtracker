package food

import (
	"context"
	"strings"
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
	if err := createMeal(svc, ctx, m); err != nil {
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
	if err := createMeal(svc, ctx, &Meal{Day: "2026-08-01", Items: []MealItem{{Name: "toast", Calories: 100, Tier: TierNeutral, FractionPct: 100}}}); err != nil {
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
	if d.Food == nil {
		t.Fatalf("Food is nil, want food totals")
	}
	if d.Food.Calories != 200 {
		t.Errorf("Calories = %d, want 200 (as-eaten)", d.Food.Calories)
	}
	if d.Food.ProteinG != 20 {
		t.Errorf("ProteinG = %v, want 20", d.Food.ProteinG)
	}
	if d.Food.QualityScore == nil || *d.Food.QualityScore != 100 {
		t.Errorf("QualityScore = %v, want 100", d.Food.QualityScore)
	}
	// protein 20 g * 4 = 80 kcal of 200 = 40.0%
	if d.Food.ProteinPctCalories != 40 {
		t.Errorf("ProteinPctCalories = %v, want 40", d.Food.ProteinPctCalories)
	}
	if d.WeightLb == nil || *d.WeightLb != 187.4 {
		t.Errorf("WeightLb = %v, want 187.4", d.WeightLb)
	}
	if d.Exercise == nil {
		t.Fatalf("Exercise is nil, want a summary")
	}
	if d.Exercise.CardioMin != 45 || d.Exercise.StrengthSessions != 1 || d.Exercise.ActiveMin != 45 {
		t.Errorf("Exercise summary = %+v", d.Exercise)
	}

	// One dinner food -> one group with one food.
	if len(doc.MealGroups) != 1 {
		t.Fatalf("MealGroups = %d, want 1", len(doc.MealGroups))
	}
	g := doc.MealGroups[0]
	if g.Slot != SlotDinner || g.OccasionIndex != 0 || g.EntryCount != 1 {
		t.Errorf("group = %+v, want dinner occasion 0 entry_count 1", g)
	}
	if g.Calories != 200 || g.ProteinG != 20 {
		t.Errorf("group as-eaten = %d kcal / %v g protein", g.Calories, g.ProteinG)
	}
	item := g.Items[0]
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

	// Coverage: single-day data inside a 31-day range.
	cov := doc.Meta.Coverage
	if cov["food"].DaysWithData != 1 || cov["food"].DaysInRange != 31 {
		t.Errorf("food coverage = %+v, want 1/31", cov["food"])
	}
	if cov["food"].FirstLogged == nil || *cov["food"].FirstLogged != "2026-07-10" {
		t.Errorf("food first_logged = %v, want 2026-07-10", cov["food"].FirstLogged)
	}
	// Food/exercise/weight all began after range start (2026-07-01) => warnings.
	foundExWarn := false
	for _, w := range doc.AnalysisWarnings {
		if strings.Contains(w, "Exercise logging began 2026-07-10") {
			foundExWarn = true
		}
	}
	if !foundExWarn {
		t.Errorf("expected an exercise-began warning, got %v", doc.AnalysisWarnings)
	}
}

// TestExportAnalysisMealGroups checks that foods logged to the same slot across
// separate logging actions land in one meal group (one container per slot), and
// entry_count is the number of foods.
func TestExportAnalysisMealGroups(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	// Breakfast built up over three separate logs — one container, three foods.
	for i := 0; i < 3; i++ {
		m := &Meal{
			Day: "2026-07-10", Slot: strPtr(SlotBreakfast),
			Items: []MealItem{{Name: "eggs", Calories: 100, ProteinMg: 10000, Tier: TierHardYes, FractionPct: 100}},
		}
		if err := createMeal(svc, ctx, m); err != nil {
			t.Fatalf("AddFoods breakfast %d: %v", i, err)
		}
	}

	doc, err := svc.ExportAnalysis(ctx, "2026-07-10", "2026-07-10")
	if err != nil {
		t.Fatalf("ExportAnalysis: %v", err)
	}
	if len(doc.MealGroups) != 1 {
		t.Fatalf("MealGroups = %d, want 1 (all breakfast foods in one container)", len(doc.MealGroups))
	}
	g := doc.MealGroups[0]
	if g.EntryCount != 3 {
		t.Errorf("EntryCount = %d, want 3 (foods)", g.EntryCount)
	}
	if g.Calories != 300 {
		t.Errorf("group Calories = %d, want 300 (3 foods summed)", g.Calories)
	}
	if len(g.Descriptions) != 3 {
		t.Errorf("Descriptions = %v, want 3 food names", g.Descriptions)
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

	if err := createMeal(svc, ctx, &Meal{Day: "2026-07-10", Items: []MealItem{{Name: "egg", Calories: 70, Tier: TierNeutral, FractionPct: 100}}}); err != nil {
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
