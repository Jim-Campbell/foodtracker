package food

import (
	"context"
	"log/slog"
	"testing"
)

func TestNormalizeFoodName(t *testing.T) {
	cases := map[string]string{
		"  Deluxe   Mixed  NUTS ": "deluxe mixed nuts",
		"Mixed nuts":              "mixed nuts",
		"Orgain Protein Powder":   "orgain protein powder",
	}
	for in, want := range cases {
		if got := NormalizeFoodName(in); got != want {
			t.Errorf("NormalizeFoodName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalFromItemBackComputesPer100(t *testing.T) {
	ref := "1"
	g := 200
	it := MealItem{
		Name: "Salmon", Grams: &g, FractionPct: 100,
		Calories: 400, ProteinMg: 50000, SatFatMg: 4000,
		Source: SourceUSDA, SourceRef: &ref, Tier: TierHardYes,
	}
	cf, ok := canonicalFromItem(it)
	if !ok {
		t.Fatal("expected the item to be eligible for accretion")
	}
	if cf.NormalizedName != "salmon" {
		t.Errorf("NormalizedName = %q, want salmon", cf.NormalizedName)
	}
	if cf.Per100g.Calories != 200 { // 400 * 100 / 200
		t.Errorf("cal per 100g = %d, want 200", cf.Per100g.Calories)
	}
	if cf.Per100g.ProteinMg != 25000 {
		t.Errorf("protein per 100g = %d, want 25000", cf.Per100g.ProteinMg)
	}
	if cf.DefaultGrams == nil || *cf.DefaultGrams != 200 {
		t.Errorf("DefaultGrams = %v, want 200", cf.DefaultGrams)
	}
}

func TestCanonicalFromItemSkipsIneligible(t *testing.T) {
	g := 100
	if _, ok := canonicalFromItem(MealItem{Name: "guess", Source: SourceAI, Grams: &g}); ok {
		t.Error("AI-estimated items should not be accreted")
	}
	if _, ok := canonicalFromItem(MealItem{Name: "manual", Source: SourceManual, Grams: &g}); ok {
		t.Error("manual items should not be accreted")
	}
	if _, ok := canonicalFromItem(MealItem{Name: "no grams", Source: SourceUSDA}); ok {
		t.Error("items without a portion weight should not be accreted")
	}
}

func TestServiceAccretesCanonicalOnSave(t *testing.T) {
	store := newFakeStore()
	svc := NewService(store, slog.Default())
	ctx := context.Background()

	ref := "168608"
	g := 170
	m := &Meal{
		Day: "2026-07-23", Slot: strPtr(SlotDinner),
		Items: []MealItem{{
			Name: "Tri-tip, grilled", Grams: &g, FractionPct: 100,
			Calories: 450, ProteinMg: 42000, FatMg: 30000, SatFatMg: 12000,
			Tier: TierSoftYes, TierReason: "lean red meat",
			Source: SourceUSDA, SourceRef: &ref, Confidence: ConfidenceHigh,
		}},
	}
	if err := createMeal(svc, ctx, m); err != nil {
		t.Fatalf("CreateMeal: %v", err)
	}

	cf := store.canonical[NormalizeFoodName("Tri-tip, grilled")]
	if cf == nil {
		t.Fatal("expected a canonical entry to be accreted on save")
	}
	if cf.Per100g.Calories != 264 { // 450 * 100 / 170
		t.Errorf("cal per 100g = %d, want 264", cf.Per100g.Calories)
	}
	if cf.Tier != TierSoftYes {
		t.Errorf("Tier = %q, want soft_yes", cf.Tier)
	}
}
