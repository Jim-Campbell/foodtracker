package food

import "fmt"

// EatenValue derives the as-eaten amount from a full-portion value. Integer
// division, multiply before divide, matching every other derived amount in
// this app.
func EatenValue(v int64, fractionPct int) int64 {
	return v * int64(fractionPct) / 100
}

// DayScore is the calorie-weighted mean of tier values (hard_yes=100,
// soft_yes=75, neutral=50, soft_no=25, hard_no=0) over the day's as-eaten
// items. Zero-calorie items (black coffee, water) don't participate in the
// weighting. Returns false when the day has no calorie-bearing items.
func DayScore(items []MealItem) (int, bool) {
	var weightedSum, totalCalories int64
	for _, it := range items {
		eaten := EatenValue(it.Calories, it.FractionPct)
		if eaten <= 0 {
			continue
		}
		weightedSum += eaten * int64(TierValue[it.Tier])
		totalCalories += eaten
	}
	if totalCalories == 0 {
		return 0, false
	}
	return int(weightedSum / totalCalories), true
}

// ValidateItem checks a meal item before it's saved. Enum and range
// violations are returned as errors (the save is rejected); a calories/macros
// mismatch (Atwater check) is returned as a warning only — the caller clamps
// confidence to low and keeps going, since AI nutrition estimates are
// approximate by nature.
func ValidateItem(it MealItem) (errs []string, warnings []string) {
	if !validTiers[it.Tier] {
		errs = append(errs, fmt.Sprintf("invalid tier: %q", it.Tier))
	}
	if !validSources[it.Source] {
		errs = append(errs, fmt.Sprintf("invalid source: %q", it.Source))
	}
	if it.Confidence != "" && !validConfidences[it.Confidence] {
		errs = append(errs, fmt.Sprintf("invalid confidence: %q", it.Confidence))
	}
	if it.FractionPct < 1 || it.FractionPct > 100 {
		errs = append(errs, "fraction_pct must be between 1 and 100")
	}
	if it.Calories < 0 || it.ProteinMg < 0 || it.CarbsMg < 0 || it.FatMg < 0 ||
		it.FiberMg < 0 || it.SatFatMg < 0 || it.SugarMg < 0 || it.SodiumMg < 0 {
		errs = append(errs, "amounts must be non-negative")
	}
	if it.Grams != nil && *it.Grams < 0 {
		errs = append(errs, "grams must be non-negative")
	}
	if len(errs) > 0 {
		return errs, warnings
	}

	// Atwater check: protein/carbs at 4 kcal/g, fat at 9 kcal/g, must land
	// within +/-30% of the stated calories. Integer math throughout (mg -> g
	// is a /1000 baked into the kcal formula below).
	if it.Calories > 0 {
		atwaterKcal := (it.ProteinMg*4 + it.CarbsMg*4 + it.FatMg*9) / 1000
		lower := it.Calories * 7 / 10
		upper := it.Calories * 13 / 10
		if atwaterKcal < lower || atwaterKcal > upper {
			warnings = append(warnings, fmt.Sprintf(
				"calories (%d) don't match macros (Atwater estimate ~%d kcal) for %q",
				it.Calories, atwaterKcal, it.Name))
		}
	}
	return errs, warnings
}
