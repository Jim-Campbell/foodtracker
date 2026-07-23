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

	if _, err := svc.UpdateSettings(ctx, &Settings{CalorieTarget: 600, ProteinTargetMg: 100000, SatFatTargetMg: 14000}); err != nil {
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

// ---- exercise ----

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestServiceCreateExerciseValidSessionsPass(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	cardio := &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Run"), DurationMin: intPtr(32)}
	if err := svc.CreateExercise(ctx, cardio); err != nil {
		t.Fatalf("cardio CreateExercise failed: %v", err)
	}

	yoga := &ExerciseSession{Day: "2026-07-07", Type: ExerciseYoga, Location: strPtr("Studio"), Style: strPtr("Vinyasa"), DurationMin: intPtr(60)}
	if err := svc.CreateExercise(ctx, yoga); err != nil {
		t.Fatalf("yoga CreateExercise failed: %v", err)
	}

	strength := &ExerciseSession{Day: "2026-07-07", Type: ExerciseStrength, Location: strPtr("Crunch")}
	if err := svc.CreateExercise(ctx, strength); err != nil {
		t.Fatalf("strength CreateExercise failed: %v", err)
	}

	meditation := &ExerciseSession{Day: "2026-07-07", Type: ExerciseMeditation, DurationMin: intPtr(10)}
	if err := svc.CreateExercise(ctx, meditation); err != nil {
		t.Fatalf("meditation CreateExercise failed: %v", err)
	}

	pt := &ExerciseSession{Day: "2026-07-07", Type: ExercisePT}
	if err := svc.CreateExercise(ctx, pt); err != nil {
		t.Fatalf("pt CreateExercise failed: %v", err)
	}
}

func TestServiceCreateExerciseRejectsMissingFields(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	cases := []struct {
		name string
		e    *ExerciseSession
	}{
		{"cardio missing activity", &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, DurationMin: intPtr(30)}},
		{"yoga missing style", &ExerciseSession{Day: "2026-07-07", Type: ExerciseYoga, Location: strPtr("Home"), DurationMin: intPtr(30)}},
		{"strength with duration", &ExerciseSession{Day: "2026-07-07", Type: ExerciseStrength, Location: strPtr("Home"), DurationMin: intPtr(45)}},
		{"meditation zero duration", &ExerciseSession{Day: "2026-07-07", Type: ExerciseMeditation, DurationMin: intPtr(0)}},
		{"pt with duration", &ExerciseSession{Day: "2026-07-07", Type: ExercisePT, DurationMin: intPtr(20)}},
	}
	for _, c := range cases {
		if err := svc.CreateExercise(ctx, c.e); err == nil {
			t.Errorf("%s: expected an error, got nil", c.name)
		}
	}
}

func TestServiceCreateExerciseHRZones(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	ok := &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Run"),
		DurationMin: intPtr(45), HRZones: map[string]int{"1": 5, "2": 20, "3": 15, "5": 5}}
	if err := svc.CreateExercise(ctx, ok); err != nil {
		t.Fatalf("cardio with hr_zones should pass: %v", err)
	}

	bad := []struct {
		name string
		e    *ExerciseSession
	}{
		{"zones on non-cardio", &ExerciseSession{Day: "2026-07-07", Type: ExerciseMeditation, DurationMin: intPtr(10), HRZones: map[string]int{"1": 5}}},
		{"invalid zone key", &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Run"), DurationMin: intPtr(45), HRZones: map[string]int{"6": 5}}},
		{"negative minutes", &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Run"), DurationMin: intPtr(45), HRZones: map[string]int{"2": -3}}},
	}
	for _, c := range bad {
		if err := svc.CreateExercise(ctx, c.e); err == nil {
			t.Errorf("%s: expected an error, got nil", c.name)
		}
	}
}

func TestServiceCreateExerciseAppendsNoDedupe(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	first := &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Run"), DurationMin: intPtr(30)}
	second := &ExerciseSession{Day: "2026-07-07", Type: ExerciseCardio, Activity: strPtr("Bike"), DurationMin: intPtr(45)}
	if err := svc.CreateExercise(ctx, first); err != nil {
		t.Fatalf("CreateExercise failed: %v", err)
	}
	if err := svc.CreateExercise(ctx, second); err != nil {
		t.Fatalf("CreateExercise failed: %v", err)
	}

	sessions, err := svc.ListExerciseRange(ctx, "2026-07-07", "2026-07-07")
	if err != nil {
		t.Fatalf("ListExerciseRange failed: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("len(sessions) = %d, want 2 (append, no dedupe)", len(sessions))
	}
}

func TestServiceListExerciseRangeInclusive(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	inside := &ExerciseSession{Day: "2026-07-02", Type: ExerciseMeditation, DurationMin: intPtr(10)}
	startEdge := &ExerciseSession{Day: "2026-07-01", Type: ExerciseMeditation, DurationMin: intPtr(10)}
	endEdge := &ExerciseSession{Day: "2026-07-03", Type: ExerciseMeditation, DurationMin: intPtr(10)}
	outside := &ExerciseSession{Day: "2026-07-04", Type: ExerciseMeditation, DurationMin: intPtr(10)}
	for _, e := range []*ExerciseSession{inside, startEdge, endEdge, outside} {
		if err := svc.CreateExercise(ctx, e); err != nil {
			t.Fatalf("CreateExercise failed: %v", err)
		}
	}

	sessions, err := svc.ListExerciseRange(ctx, "2026-07-01", "2026-07-03")
	if err != nil {
		t.Fatalf("ListExerciseRange failed: %v", err)
	}
	if len(sessions) != 3 {
		t.Errorf("len(sessions) = %d, want 3 (inclusive of both ends, excluding 07-04)", len(sessions))
	}
}

func TestServiceUpdateSettingsRoundTripsExerciseTargets(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	updated, err := svc.UpdateSettings(ctx, &Settings{
		CalorieTarget: 1800, ProteinTargetMg: 165000, SatFatTargetMg: 14000,
		CardioWeeklyTarget: 4, StrengthWeeklyTarget: 3, YogaWeeklyTarget: 1, MeditationWeeklyDays: 5, PTWeeklyDays: 6,
	})
	if err != nil {
		t.Fatalf("UpdateSettings failed: %v", err)
	}
	if updated.CardioWeeklyTarget != 4 || updated.StrengthWeeklyTarget != 3 || updated.YogaWeeklyTarget != 1 || updated.MeditationWeeklyDays != 5 || updated.PTWeeklyDays != 6 {
		t.Errorf("weekly targets did not round-trip: %+v", updated)
	}
}

func TestServiceUpdateSettingsRejectsNegativeExerciseTarget(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()
	_, err := svc.UpdateSettings(ctx, &Settings{
		CalorieTarget: 1800, ProteinTargetMg: 165000, SatFatTargetMg: 14000, CardioWeeklyTarget: -1,
	})
	if err == nil {
		t.Error("expected an error for a negative cardio_weekly_target, got nil")
	}
}
