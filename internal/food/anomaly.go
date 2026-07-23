package food

import (
	"fmt"
	"sort"
)

// scoreDropThreshold is how far a day's quality score must fall below its own
// trailing baseline before the day is flagged as an anomaly (item 6). The
// score is an anomaly detector, not a daily grade — small day-to-day wobble
// shouldn't trip it, so the bar is a clear 10-point drop.
const scoreDropThreshold = 10

// minBaselineDays is the fewest prior scored days needed before a trailing
// baseline is trustworthy enough to call a day an outlier against.
const minBaselineDays = 3

// ComputeAnomaly attributes an off day to the specific foods that drove it.
// It fires when the day's score falls scoreDropThreshold below its trailing
// baseline, and/or the day's saturated fat exceeds the ceiling. It returns nil
// on a normal day so the UI shows nothing. items are the day's full-portion
// MealItems; baseline is the trailing mean score (nil when too few prior days).
func ComputeAnomaly(items []MealItem, score, baseline *int, satFatMg, satTargetMg int64) *DayAnomaly {
	a := &DayAnomaly{}

	// Saturated fat over the ceiling — the metric most likely to drive an off
	// day given Jim's cardiovascular profile, so it leads.
	if satTargetMg > 0 && satFatMg > satTargetMg {
		a.Headline = fmt.Sprintf("Saturated fat %.0f g — over your %.0f g ceiling", mgToG(satFatMg), mgToG(satTargetMg))
		for _, r := range topSatFatItems(items, 3) {
			a.Reasons = append(a.Reasons, r)
		}
	}

	// Quality score well below the personal trailing baseline.
	if score != nil && baseline != nil && *score <= *baseline-scoreDropThreshold {
		head := fmt.Sprintf("Quality %d — below your recent average of %d", *score, *baseline)
		if a.Headline == "" {
			a.Headline = head
		} else {
			a.Headline += "; " + head
		}
		for _, r := range topDragItems(items, 3) {
			a.Reasons = append(a.Reasons, r)
		}
	}

	if a.Headline == "" {
		return nil
	}
	return a
}

// topSatFatItems returns up to n reason lines for the day's biggest saturated-
// fat contributors (as-eaten), largest first.
func topSatFatItems(items []MealItem, n int) []string {
	type row struct {
		name  string
		satMg int64
	}
	var rows []row
	for _, it := range items {
		sat := EatenValue(it.SatFatMg, it.FractionPct)
		if sat <= 0 {
			continue
		}
		rows = append(rows, row{it.Name, sat})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].satMg > rows[j].satMg })

	var out []string
	for i := 0; i < len(rows) && i < n; i++ {
		out = append(out, fmt.Sprintf("%s — %.1f g sat fat", rows[i].name, mgToG(rows[i].satMg)))
	}
	return out
}

// topDragItems returns up to n reason lines for the calorie-bearing items that
// pulled the day's score down most — ranked by as-eaten calories weighted by
// how far below neutral their tier sits. Only below-neutral items (soft_no /
// hard_no) can be a drag; a low score built entirely of neutral items has no
// single culprit and yields no reasons.
func topDragItems(items []MealItem, n int) []string {
	type row struct {
		name string
		tier string
		cal  int64
		drag int64
	}
	var rows []row
	for _, it := range items {
		cal := EatenValue(it.Calories, it.FractionPct)
		if cal <= 0 {
			continue
		}
		deficit := int64(TierValue[TierNeutral] - TierValue[it.Tier]) // >0 only for soft_no/hard_no
		if deficit <= 0 {
			continue
		}
		rows = append(rows, row{it.Name, it.Tier, cal, cal * deficit})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].drag > rows[j].drag })

	var out []string
	for i := 0; i < len(rows) && i < n; i++ {
		out = append(out, fmt.Sprintf("%s — %s, %d cal", rows[i].name, tierLabel(rows[i].tier), rows[i].cal))
	}
	return out
}

// tierLabel renders a tier constant as the human label used in the diet
// framework and the PWA legend.
func tierLabel(tier string) string {
	switch tier {
	case TierHardYes:
		return "Hard Yes"
	case TierSoftYes:
		return "Soft Yes"
	case TierNeutral:
		return "Neutral"
	case TierSoftNo:
		return "Soft No"
	case TierHardNo:
		return "Hard No"
	}
	return tier
}
