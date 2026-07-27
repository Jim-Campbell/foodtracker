package food

import (
	"context"
	"log/slog"
	"testing"
)

func gp(g int) *int { return &g }

func slotPtr(s string) *string { return &s }

func greensTag() ComponentTag {
	return ComponentTag{NormalizedName: "kale", ComponentID: ComponentLeafyGreens, GramsPerServing: 30}
}

func TestComputeInclusionStraightCount(t *testing.T) {
	// 3 logged cups of greens at 30 g/serving, spread across three containers
	// so the per-meal cap doesn't interfere -> ServingsX100 == 300.
	tags := []ComponentTag{{NormalizedName: "spinach", ComponentID: ComponentLeafyGreens, GramsPerServing: 30}}
	meals := []Meal{
		{Day: "2026-01-10", Slot: slotPtr("breakfast"), Items: []MealItem{{Name: "spinach", Grams: gp(30), FractionPct: 100}}},
		{Day: "2026-01-11", Slot: slotPtr("lunch"), Items: []MealItem{{Name: "spinach", Grams: gp(30), FractionPct: 100}}},
		{Day: "2026-01-12", Slot: slotPtr("dinner"), Items: []MealItem{{Name: "spinach", Grams: gp(30), FractionPct: 100}}},
	}
	w := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", nil)
	got := progressFor(w, ComponentLeafyGreens)
	if got.ServingsX100 != 300 {
		t.Errorf("ServingsX100 = %d, want 300", got.ServingsX100)
	}
}

func TestComputeInclusionFractionPctScaling(t *testing.T) {
	// 200g of a 100g/serving food at fraction_pct 50 -> 100g eaten -> 1.00 serving.
	tags := []ComponentTag{{NormalizedName: "lentil soup", ComponentID: ComponentLegumes, GramsPerServing: 100}}
	meals := []Meal{
		{Day: "2026-01-12", Items: []MealItem{{Name: "lentil soup", Grams: gp(200), FractionPct: 50}}},
	}
	w := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", nil)
	got := progressFor(w, ComponentLegumes)
	if got.ServingsX100 != 100 {
		t.Errorf("ServingsX100 = %d, want 100", got.ServingsX100)
	}
}

func TestComputeInclusionPerMealCap(t *testing.T) {
	tags := []ComponentTag{greensTag()}
	// 400g in one slot -> raw 1333, capped to 200, not 1333.
	oneSlot := []Meal{
		{Day: "2026-01-12", Slot: slotPtr("dinner"), Items: []MealItem{{Name: "kale", Grams: gp(400), FractionPct: 100}}},
	}
	w := ComputeInclusion(oneSlot, tags, "2026-01-06", "2026-01-12", nil)
	got := progressFor(w, ComponentLeafyGreens)
	if got.ServingsX100 != 200 {
		t.Errorf("single-slot ServingsX100 = %d, want 200 (capped)", got.ServingsX100)
	}

	// Two slots each over the cap -> 400 total (200 + 200).
	twoSlots := []Meal{
		{Day: "2026-01-12", Slot: slotPtr("lunch"), Items: []MealItem{{Name: "kale", Grams: gp(400), FractionPct: 100}}},
		{Day: "2026-01-12", Slot: slotPtr("dinner"), Items: []MealItem{{Name: "kale", Grams: gp(400), FractionPct: 100}}},
	}
	w2 := ComputeInclusion(twoSlots, tags, "2026-01-06", "2026-01-12", nil)
	got2 := progressFor(w2, ComponentLeafyGreens)
	if got2.ServingsX100 != 400 {
		t.Errorf("two-slot ServingsX100 = %d, want 400", got2.ServingsX100)
	}
}

func TestComputeInclusionProgressCap(t *testing.T) {
	// 10 servings of berries (target 5) spread across 5 containers at exactly
	// 2.00 servings each (at, not over, the per-meal cap) -> ProgressPct == 100,
	// and the score still reflects zeros elsewhere (breadth is the point).
	tags := []ComponentTag{{NormalizedName: "berries", ComponentID: ComponentBerries, GramsPerServing: 75}}
	var meals []Meal
	slots := []string{"breakfast", "lunch", "dinner", "snack", "breakfast"}
	for i, slot := range slots {
		day := "2026-01-1" + string(rune('0'+i))
		meals = append(meals, Meal{Day: day, Slot: slotPtr(slot), Items: []MealItem{
			{Name: "berries", Grams: gp(150), FractionPct: 100}, // 2.00 servings
		}})
	}
	w := ComputeInclusion(meals, tags, "2026-01-10", "2026-01-14", nil)
	berries := progressFor(w, ComponentBerries)
	if berries.ProgressPct != 100 {
		t.Errorf("berries ProgressPct = %d, want 100", berries.ProgressPct)
	}
	greens := progressFor(w, ComponentLeafyGreens)
	if greens.ProgressPct != 0 {
		t.Errorf("leafy_greens ProgressPct = %d, want 0", greens.ProgressPct)
	}
	// score = (100 + 0 + 0 + 0 + 0) / 5 = 20
	if w.Score != 20 {
		t.Errorf("Score = %d, want 20", w.Score)
	}
}

func TestComputeInclusionWindowEdges(t *testing.T) {
	tags := []ComponentTag{{NormalizedName: "lentils", ComponentID: ComponentLegumes, GramsPerServing: 90}}
	// end = 2026-01-15, window = 2026-01-09..2026-01-15 (end-6 .. end).
	meals := []Meal{
		{Day: "2026-01-09", Items: []MealItem{{Name: "lentils", Grams: gp(90), FractionPct: 100}}}, // end-6: counts, expiring
		{Day: "2026-01-08", Items: []MealItem{{Name: "lentils", Grams: gp(90), FractionPct: 100}}}, // end-7: doesn't count at all
	}
	w := ComputeInclusion(meals, tags, "2026-01-09", "2026-01-15", nil)
	legumes := progressFor(w, ComponentLegumes)
	if legumes.ServingsX100 != 100 {
		t.Errorf("ServingsX100 = %d, want 100 (only the end-6 serving should count)", legumes.ServingsX100)
	}
	if legumes.ExpiringWhole != 1 {
		t.Errorf("ExpiringWhole = %d, want 1", legumes.ExpiringWhole)
	}
}

func TestComputeInclusionUntaggedAndUnmeasured(t *testing.T) {
	tags := []ComponentTag{greensTag()}
	meals := []Meal{
		{Day: "2026-01-12", Items: []MealItem{
			{Name: "kale", Grams: nil, FractionPct: 100},           // unmeasured: no grams
			{Name: "white rice", Grams: gp(150), FractionPct: 100}, // untagged: contributes nothing, not unmeasured
		}},
	}
	w := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", nil)
	if w.UnmeasuredFoods != 1 {
		t.Errorf("UnmeasuredFoods = %d, want 1", w.UnmeasuredFoods)
	}
	greens := progressFor(w, ComponentLeafyGreens)
	if greens.ServingsX100 != 0 {
		t.Errorf("ServingsX100 = %d, want 0 (the kale had no grams)", greens.ServingsX100)
	}
}

func TestComputeInclusionMultiTag(t *testing.T) {
	// Kale credits both leafy_greens and cruciferous.
	tags := []ComponentTag{
		{NormalizedName: "kale", ComponentID: ComponentLeafyGreens, GramsPerServing: 30},
		{NormalizedName: "kale", ComponentID: ComponentCruciferous, GramsPerServing: 85},
	}
	meals := []Meal{
		{Day: "2026-01-12", Items: []MealItem{{Name: "kale", Grams: gp(85), FractionPct: 100}}},
	}
	w := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", nil)
	greens := progressFor(w, ComponentLeafyGreens)
	cruciferous := progressFor(w, ComponentCruciferous)
	if greens.ServingsX100 <= 0 {
		t.Errorf("leafy_greens ServingsX100 = %d, want > 0", greens.ServingsX100)
	}
	if cruciferous.ServingsX100 != 100 {
		t.Errorf("cruciferous ServingsX100 = %d, want 100", cruciferous.ServingsX100)
	}
}

func TestComputeInclusionAllZeroDay(t *testing.T) {
	w := ComputeInclusion(nil, nil, "2026-01-06", "2026-01-12", nil)
	if w.Score != 0 {
		t.Errorf("Score = %d, want 0 (empty window is a real answer, not an error)", w.Score)
	}
	if len(w.Components) != len(ComponentCatalog) {
		t.Errorf("len(Components) = %d, want %d", len(w.Components), len(ComponentCatalog))
	}
}

// Regression: this phase must not change the calorie-weighted composition
// score at all.
func TestComputeInclusionDoesNotAffectDayScore(t *testing.T) {
	items := []MealItem{
		{Calories: 500, FractionPct: 100, Tier: TierHardYes},
		{Calories: 300, FractionPct: 100, Tier: TierSoftNo},
		{Calories: 200, FractionPct: 100, Tier: TierSoftYes},
	}
	score, ok := DayScore(items)
	if !ok || score != 72 {
		t.Errorf("DayScore = (%d, %v), want (72, true) -- composition score must be unchanged", score, ok)
	}
}

// TestNudgeSelectionPicksLargestRelativeGap is the worked example from
// inclusion-5-nudges.md's Tests section: berries at 1/5 (gap 0.8) beats
// greens at 6/7 (gap 0.14), with the other three components at target so they
// can't interfere.
func TestNudgeSelectionPicksLargestRelativeGap(t *testing.T) {
	tags := []ComponentTag{
		{NormalizedName: "berries", ComponentID: ComponentBerries, GramsPerServing: 75},
		{NormalizedName: "spinach", ComponentID: ComponentLeafyGreens, GramsPerServing: 30},
		{NormalizedName: "lentils", ComponentID: ComponentLegumes, GramsPerServing: 90},
		{NormalizedName: "salmon", ComponentID: ComponentFattyFish, GramsPerServing: 100},
		{NormalizedName: "broccoli", ComponentID: ComponentCruciferous, GramsPerServing: 85},
	}
	day := "2026-02-05"
	meals := []Meal{
		{Day: day, Slot: slotPtr("s1"), Items: []MealItem{{Name: "berries", Grams: gp(75), FractionPct: 100}}}, // 1/5 berries -> gap 0.8
		{Day: day, Slot: slotPtr("s2"), Items: []MealItem{{Name: "spinach", Grams: gp(60), FractionPct: 100}}}, // greens: 3x2.00 = 6/7 -> gap 0.14
		{Day: day, Slot: slotPtr("s3"), Items: []MealItem{{Name: "spinach", Grams: gp(60), FractionPct: 100}}},
		{Day: day, Slot: slotPtr("s4"), Items: []MealItem{{Name: "spinach", Grams: gp(60), FractionPct: 100}}},
		{Day: day, Slot: slotPtr("s5"), Items: []MealItem{{Name: "lentils", Grams: gp(180), FractionPct: 100}}}, // legumes met: 5/5
		{Day: day, Slot: slotPtr("s6"), Items: []MealItem{{Name: "lentils", Grams: gp(180), FractionPct: 100}}},
		{Day: day, Slot: slotPtr("s7"), Items: []MealItem{{Name: "lentils", Grams: gp(90), FractionPct: 100}}},
		{Day: day, Slot: slotPtr("s8"), Items: []MealItem{{Name: "salmon", Grams: gp(150), FractionPct: 100}}}, // fatty fish met: 3/3
		{Day: day, Slot: slotPtr("s9"), Items: []MealItem{{Name: "salmon", Grams: gp(150), FractionPct: 100}}},
		{Day: day, Slot: slotPtr("s10"), Items: []MealItem{{Name: "broccoli", Grams: gp(170), FractionPct: 100}}}, // cruciferous met: 4/4
		{Day: day, Slot: slotPtr("s11"), Items: []MealItem{{Name: "broccoli", Grams: gp(170), FractionPct: 100}}},
	}
	w := ComputeInclusion(meals, tags, "2026-01-30", "2026-02-05", nil)

	needs := SupplyNeeds(w)
	if len(needs) != 2 {
		t.Fatalf("SupplyNeeds = %d entries, want 2 (only berries and greens miss target): %+v", len(needs), needs)
	}
	if needs[0].ID != ComponentBerries {
		t.Errorf("SupplyNeeds[0].ID = %s, want berries (relative gap 0.8 beats greens' 0.14)", needs[0].ID)
	}

	dec := DecisionCandidate(w)
	if dec == nil || dec.ID != ComponentBerries {
		t.Errorf("DecisionCandidate = %+v, want berries", dec)
	}
}

// TestDecisionCandidateTieBreaksTowardExpiring: two components tied on
// relative gap -- the one with a serving aging out within 24h wins, since
// that's the one where today's action prevents a loss (spec §7).
func TestDecisionCandidateTieBreaksTowardExpiring(t *testing.T) {
	w := InclusionWindow{Components: []ComponentProgress{
		{ID: ComponentLegumes, Target: 5, ServingsX100: 250, ExpiringWhole: 0},
		{ID: ComponentBerries, Target: 5, ServingsX100: 250, ExpiringWhole: 1},
	}}
	dec := DecisionCandidate(w)
	if dec == nil || dec.ID != ComponentBerries {
		t.Errorf("DecisionCandidate = %+v, want berries (equal gap, but its serving expires within 24h)", dec)
	}
}

// TestDecisionCandidateNeverPicksComponentAtTarget covers the "never fire for
// a component at target" suppression rule directly. The complementary
// "never more than once per day" rule is enforced client-side via
// localStorage in the PWA (nudgeFor/decisionNudgeLine in pwa/index.html) --
// it depends on wall-clock date, which is a device-local, not a pure,
// concern, so it isn't expressible as a Go unit test.
func TestDecisionCandidateNeverPicksComponentAtTarget(t *testing.T) {
	w := InclusionWindow{Components: []ComponentProgress{
		{ID: ComponentBerries, Target: 5, ServingsX100: 500, Met: true},
		{ID: ComponentLeafyGreens, Target: 7, ServingsX100: 700, Met: true},
	}}
	if dec := DecisionCandidate(w); dec != nil {
		t.Errorf("DecisionCandidate = %+v, want nil (every component is at target)", dec)
	}
	if needs := SupplyNeeds(w); len(needs) != 0 {
		t.Errorf("SupplyNeeds = %+v, want empty (every component is at target)", needs)
	}
}

// TestComputeInclusionTargetOverride: a Settings.ComponentTargets override
// changes both the dot row length (Target) and whether a given count reaches
// it (Met), which together drive the score.
func TestComputeInclusionTargetOverride(t *testing.T) {
	tags := []ComponentTag{{NormalizedName: "berries", ComponentID: ComponentBerries, GramsPerServing: 75}}
	meals := []Meal{
		// 2.00 + 1.00 = 3.00 servings, split across meals to stay clear of the
		// per-meal cap.
		{Day: "2026-01-12", Slot: slotPtr("a"), Items: []MealItem{{Name: "berries", Grams: gp(150), FractionPct: 100}}},
		{Day: "2026-01-12", Slot: slotPtr("b"), Items: []MealItem{{Name: "berries", Grams: gp(75), FractionPct: 100}}},
	}

	def := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", nil)
	berries := progressFor(def, ComponentBerries)
	if berries.Target != 5 || berries.Met {
		t.Fatalf("default berries = %+v, want Target 5, not Met (3/5)", berries)
	}

	overridden := ComputeInclusion(meals, tags, "2026-01-06", "2026-01-12", map[string]int{"berries": 3})
	berriesOverridden := progressFor(overridden, ComponentBerries)
	if berriesOverridden.Target != 3 || !berriesOverridden.Met {
		t.Fatalf("overridden berries = %+v, want Target 3, Met (3/3)", berriesOverridden)
	}
	if overridden.Score <= def.Score {
		t.Errorf("Score with override = %d, want > default Score = %d", overridden.Score, def.Score)
	}
}

func TestSanitizeComponentTargets(t *testing.T) {
	got := SanitizeComponentTargets(map[string]int{
		ComponentBerries:     3,
		"nuts_seeds":         5,  // unknown -- dropped
		ComponentFattyFish:   0,  // out of range -- dropped
		ComponentLeafyGreens: 22, // out of range -- dropped
		ComponentLegumes:     21, // in range -- kept
	})
	if len(got) != 2 {
		t.Fatalf("SanitizeComponentTargets = %+v, want 2 entries (berries, legumes)", got)
	}
	if got[ComponentBerries] != 3 || got[ComponentLegumes] != 21 {
		t.Errorf("SanitizeComponentTargets = %+v, want berries=3, legumes=21", got)
	}
	if got := SanitizeComponentTargets(nil); got != nil {
		t.Errorf("SanitizeComponentTargets(nil) = %+v, want nil", got)
	}
}

func progressFor(w InclusionWindow, id string) ComponentProgress {
	for _, c := range w.Components {
		if c.ID == id {
			return c
		}
	}
	return ComponentProgress{}
}

func TestSanitizeItemComponents(t *testing.T) {
	got := SanitizeItemComponents([]ItemComponent{
		{ComponentID: ComponentLeafyGreens, GramsPerServing: 30},
		{ComponentID: "nuts_seeds", GramsPerServing: 40},          // unknown -- dropped
		{ComponentID: ComponentLeafyGreens, GramsPerServing: 999}, // duplicate -- collapsed to first
		{ComponentID: ComponentBerries, GramsPerServing: 0},       // clamps up to 1
		{ComponentID: ComponentFattyFish, GramsPerServing: 5000},  // clamps down to 2000
	})
	if len(got) != 3 {
		t.Fatalf("SanitizeItemComponents = %+v, want 3 entries (greens once, berries, fatty_fish)", got)
	}
	byID := map[string]ItemComponent{}
	for _, c := range got {
		byID[c.ComponentID] = c
	}
	if byID[ComponentLeafyGreens].GramsPerServing != 30 {
		t.Errorf("leafy_greens grams = %d, want 30 (first occurrence kept)", byID[ComponentLeafyGreens].GramsPerServing)
	}
	if byID[ComponentBerries].GramsPerServing != 1 {
		t.Errorf("berries grams = %d, want clamped to 1", byID[ComponentBerries].GramsPerServing)
	}
	if byID[ComponentFattyFish].GramsPerServing != 2000 {
		t.Errorf("fatty_fish grams = %d, want clamped to 2000", byID[ComponentFattyFish].GramsPerServing)
	}
	if _, ok := byID["nuts_seeds"]; ok {
		t.Error("unknown component_id should have been dropped, not stored")
	}
	if got := SanitizeItemComponents(nil); got != nil {
		t.Errorf("SanitizeItemComponents(nil) = %+v, want nil", got)
	}
}

// TestAccreteComponentTagsRespectsUserSource: an AI-sourced proposal on a
// later save must never overwrite a food's user-corrected tag (build prompt
// §2's "never overwritten" invariant) -- the user row's grams should survive
// even though the second save's AI proposal has a different value.
func TestAccreteComponentTagsRespectsUserSource(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := NewService(store, slog.Default())

	if _, err := svc.AddFoods(ctx, "2026-01-10", slotPtr("lunch"), []MealItem{
		{Name: "kale salad", Grams: gp(60), FractionPct: 100, Tier: TierHardYes,
			Components: []ItemComponent{{ComponentID: ComponentLeafyGreens, GramsPerServing: 45, Source: TagSourceUser}}},
	}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if _, err := svc.AddFoods(ctx, "2026-01-11", slotPtr("dinner"), []MealItem{
		{Name: "kale salad", Grams: gp(60), FractionPct: 100, Tier: TierHardYes,
			Components: []ItemComponent{{ComponentID: ComponentLeafyGreens, GramsPerServing: 30}}}, // ai proposal, no source
	}); err != nil {
		t.Fatalf("second save: %v", err)
	}

	tags, err := svc.ListComponentTags(ctx)
	if err != nil {
		t.Fatalf("ListComponentTags: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("tags = %+v, want exactly 1 (no duplicate row)", tags)
	}
	if tags[0].TagSource != TagSourceUser {
		t.Errorf("TagSource = %q, want %q to survive the later ai proposal", tags[0].TagSource, TagSourceUser)
	}
	if tags[0].GramsPerServing != 45 {
		t.Errorf("GramsPerServing = %d, want 45 (the user's value, not the ai proposal's 30)", tags[0].GramsPerServing)
	}
}

func TestServiceInclusionWindowAndWeeks(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	svc := NewService(store, slog.Default())

	if err := store.SetComponentTags(ctx, "kale", nil, []ComponentTag{{ComponentID: ComponentLeafyGreens, GramsPerServing: 30}}, TagSourceUser); err != nil {
		t.Fatalf("seed tag: %v", err)
	}
	if _, err := store.AddFoods(ctx, "2026-01-12", slotPtr("dinner"), []MealItem{
		{Name: "kale", Grams: gp(60), FractionPct: 100},
	}); err != nil {
		t.Fatalf("seed meal: %v", err)
	}

	win, err := svc.InclusionWindow(ctx, "2026-01-12")
	if err != nil {
		t.Fatalf("InclusionWindow: %v", err)
	}
	if win.Start != "2026-01-06" || win.End != "2026-01-12" {
		t.Errorf("window = [%s, %s], want [2026-01-06, 2026-01-12]", win.Start, win.End)
	}
	greens := progressFor(*win, ComponentLeafyGreens)
	if greens.ServingsX100 != 200 {
		t.Errorf("ServingsX100 = %d, want 200", greens.ServingsX100)
	}

	weeks, err := svc.InclusionWeeks(ctx, "2026-01-01", "2026-01-18")
	if err != nil {
		t.Fatalf("InclusionWeeks: %v", err)
	}
	// 2026-01-12 is a Monday; the completed week Mon 01-12..Sun 01-18 should
	// appear (newest first).
	if len(weeks) == 0 {
		t.Fatal("expected at least one completed week")
	}
	if weeks[0].WeekStart != "2026-01-12" {
		t.Errorf("weeks[0].WeekStart = %s, want 2026-01-12", weeks[0].WeekStart)
	}
}
