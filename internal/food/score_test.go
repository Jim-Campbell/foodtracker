package food

import "testing"

func TestEatenValue(t *testing.T) {
	cases := []struct {
		v           int64
		fractionPct int
		want        int64
	}{
		{610, 50, 305},
		{610, 33, 201}, // integer division truncates 201.3 -> 201
		{610, 100, 610},
		{0, 50, 0},
	}
	for _, c := range cases {
		if got := EatenValue(c.v, c.fractionPct); got != c.want {
			t.Errorf("EatenValue(%d, %d) = %d, want %d", c.v, c.fractionPct, got, c.want)
		}
	}
}

func TestDayScoreWorkedExample(t *testing.T) {
	// 500 kcal salmon+greens dinner (hard_yes) + 300 kcal white-flour roll
	// (soft_no) + 200 kcal Greek yogurt (soft_yes) -> 72.
	items := []MealItem{
		{Calories: 500, FractionPct: 100, Tier: TierHardYes},
		{Calories: 300, FractionPct: 100, Tier: TierSoftNo},
		{Calories: 200, FractionPct: 100, Tier: TierSoftYes},
	}
	score, ok := DayScore(items)
	if !ok {
		t.Fatal("DayScore returned ok=false, want true")
	}
	if score != 72 {
		t.Errorf("DayScore = %d, want 72", score)
	}
}

func TestDayScoreZeroCalorieOnly(t *testing.T) {
	items := []MealItem{
		{Calories: 0, FractionPct: 100, Tier: TierNeutral}, // black coffee
		{Calories: 0, FractionPct: 100, Tier: TierHardYes}, // water
	}
	_, ok := DayScore(items)
	if ok {
		t.Error("DayScore returned ok=true for a day with no calorie-bearing items, want false")
	}
}

func TestDayScoreExcludesZeroCalorieItems(t *testing.T) {
	// A zero-calorie item shouldn't drag the average toward its tier value.
	items := []MealItem{
		{Calories: 0, FractionPct: 100, Tier: TierHardNo}, // shouldn't count
		{Calories: 400, FractionPct: 100, Tier: TierHardYes},
	}
	score, ok := DayScore(items)
	if !ok {
		t.Fatal("DayScore returned ok=false, want true")
	}
	if score != 100 {
		t.Errorf("DayScore = %d, want 100 (zero-calorie item should be excluded)", score)
	}
}

func TestValidateItemEnumErrors(t *testing.T) {
	it := MealItem{Tier: "bogus", Source: "bogus", Confidence: "bogus", FractionPct: 100}
	errs, warnings := ValidateItem(it)
	if len(errs) == 0 {
		t.Error("expected errors for invalid enums, got none")
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when errors are present, got %v", warnings)
	}
}

func TestValidateItemNegativeAmountError(t *testing.T) {
	it := MealItem{Tier: TierNeutral, Source: SourceManual, Confidence: ConfidenceMedium, FractionPct: 100, Calories: -5}
	errs, _ := ValidateItem(it)
	if len(errs) == 0 {
		t.Error("expected an error for a negative amount, got none")
	}
}

func TestValidateItemAtwaterWarning(t *testing.T) {
	// Claims 1000 kcal but macros (100g carbs = 400 kcal, nothing else) only
	// support ~400 kcal -- well outside +/-30% of 1000.
	it := MealItem{
		Tier: TierNeutral, Source: SourceManual, Confidence: ConfidenceMedium, FractionPct: 100,
		Calories: 1000, CarbsMg: 100000,
	}
	errs, warnings := ValidateItem(it)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if len(warnings) == 0 {
		t.Error("expected an Atwater mismatch warning, got none")
	}
}

func TestValidateItemNoWarningWhenMacrosMatch(t *testing.T) {
	// 30g protein (120) + 50g carbs (200) + 10g fat (90) = 410 kcal, close to
	// the stated 400.
	it := MealItem{
		Tier: TierNeutral, Source: SourceManual, Confidence: ConfidenceMedium, FractionPct: 100,
		Calories: 400, ProteinMg: 30000, CarbsMg: 50000, FatMg: 10000,
	}
	errs, warnings := ValidateItem(it)
	if len(errs) != 0 {
		t.Errorf("expected no errors, got %v", errs)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings, got %v", warnings)
	}
}
