package food

import (
	"strings"
	"testing"
)

func item(name, tier string, cal, satMg int64) MealItem {
	return MealItem{Name: name, Tier: tier, Calories: cal, SatFatMg: satMg, FractionPct: 100}
}

func TestComputeAnomalyNormalDay(t *testing.T) {
	score, baseline := 80, 82
	items := []MealItem{item("salmon", TierHardYes, 400, 2000)}
	// Score near baseline, sat fat under the ceiling -> no anomaly.
	if a := ComputeAnomaly(items, &score, &baseline, 2000, 14000); a != nil {
		t.Errorf("expected no anomaly on a normal day, got %+v", a)
	}
}

func TestComputeAnomalySatFatOverCeiling(t *testing.T) {
	score, baseline := 78, 80
	items := []MealItem{
		item("Tri-tip", TierSoftYes, 450, 12000),
		item("Corn tortillas", TierSoftYes, 160, 8000),
		item("Side salad", TierHardYes, 40, 0),
	}
	a := ComputeAnomaly(items, &score, &baseline, 20000, 14000)
	if a == nil {
		t.Fatal("expected a sat-fat anomaly")
	}
	if !strings.Contains(a.Headline, "20 g") || !strings.Contains(a.Headline, "14 g") {
		t.Errorf("headline = %q, want it to name 20 g over the 14 g ceiling", a.Headline)
	}
	if len(a.Reasons) != 2 { // only two items carry sat fat
		t.Fatalf("Reasons = %v, want 2 sat-fat contributors", a.Reasons)
	}
	if !strings.HasPrefix(a.Reasons[0], "Tri-tip") {
		t.Errorf("top contributor = %q, want Tri-tip (largest sat fat)", a.Reasons[0])
	}
}

func TestComputeAnomalyLowScore(t *testing.T) {
	score, baseline := 60, 80 // 20 below baseline, past the 10 threshold
	items := []MealItem{
		item("Ice cream", TierSoftNo, 500, 9000),
		item("White roll", TierHardNo, 200, 500),
		item("Greek yogurt", TierSoftYes, 150, 1000),
	}
	// sat fat 10.5 g is under the 14 g ceiling, so only the score fires.
	a := ComputeAnomaly(items, &score, &baseline, 10500, 14000)
	if a == nil {
		t.Fatal("expected a low-score anomaly")
	}
	if !strings.Contains(a.Headline, "Quality 60") || !strings.Contains(a.Headline, "80") {
		t.Errorf("headline = %q, want it to compare 60 against the 80 baseline", a.Headline)
	}
	// hard_no (deficit 50) * 200 cal = 10000 drag vs soft_no (deficit 25) * 500
	// = 12500 -> ice cream drags most, then the roll. Yogurt is above neutral.
	if len(a.Reasons) != 2 {
		t.Fatalf("Reasons = %v, want the two below-neutral items", a.Reasons)
	}
	if !strings.HasPrefix(a.Reasons[0], "Ice cream") {
		t.Errorf("top drag = %q, want Ice cream", a.Reasons[0])
	}
}

func TestComputeAnomalyNoBaselineNoScoreFlag(t *testing.T) {
	score := 40
	items := []MealItem{item("Fried thing", TierHardNo, 600, 3000)}
	// No baseline yet (nil) -> the score can't be called an outlier, and sat fat
	// is under the ceiling, so nothing fires even though the score is low.
	if a := ComputeAnomaly(items, &score, nil, 3000, 14000); a != nil {
		t.Errorf("expected no anomaly without a baseline, got %+v", a)
	}
}
