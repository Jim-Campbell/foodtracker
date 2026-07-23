package food

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// The analysis export (GET /api/export/analysis) is a reshaped, date-ranged
// view of the log built for feeding into an LLM (Claude) chat — distinct from
// the raw full-DB backup (GET /api/export). It differs from the backup in
// ways that make it a better analytical substrate:
//
//   - Human-standard units: macros in grams, weight in pounds, calories in
//     kcal, sodium in milligrams — not the DB's integer-milligram storage.
//   - As-eaten values: the portion fraction is applied, so numbers reflect
//     what was actually consumed, not the full-portion figures on disk.
//   - No noise: ai_raw, the per-nutrient micros blob, source_ref, and
//     confidence are dropped; the quality tier + reason are kept.
//   - The meal group, not the logging entry, is the analytical unit (see
//     MealGroups). Counting entries overstates how often Jim eats — one meal
//     is sometimes one entry with three foods, sometimes three entries.
//   - Missing data is null, never 0. A metric that wasn't logged in a domain
//     on a given day is null; meta.coverage and analysis_warnings make gaps
//     impossible to average over silently.
//
// It leads with a denormalized per-day rollup (Days) — the compact, low-token
// part most analyses need — then the grouped MealGroups, the raw per-entry
// detail (Entries, for debugging), full exercise sessions (Exercise), and
// weigh-ins (Weights).
type AnalysisExport struct {
	Meta             AnalysisMeta        `json:"meta"`
	AnalysisWarnings []string            `json:"analysis_warnings"`
	Days             []AnalysisDay       `json:"days"`
	MealGroups       []AnalysisMealGroup `json:"meal_groups"`
	Entries          []AnalysisEntry     `json:"entries"`
	Exercise         []AnalysisSession   `json:"exercise"`
	Weights          []AnalysisWeight    `json:"weights"`
}

type AnalysisMeta struct {
	App                            string                      `json:"app"`
	ExportedAt                     time.Time                   `json:"exported_at"`
	RangeStart                     string                      `json:"range_start"`
	RangeEnd                       string                      `json:"range_end"`
	Units                          map[string]string           `json:"units"`
	QualityScore                   string                      `json:"quality_score"`
	Coverage                       map[string]AnalysisCoverage `json:"coverage"`
	MealGroupsAreTheAnalyticalUnit string                      `json:"meal_groups_are_the_analytical_unit"`
	Notes                          string                      `json:"notes"`
}

// AnalysisCoverage records, per data domain (food/exercise/weight), when that
// domain's logging began and how many days in the requested range actually
// carry data. It exists so no consumer can compute an average over a domain
// that wasn't being logged for part of the window without the gap being
// visible. FirstLogged is null when the domain has no data in range.
type AnalysisCoverage struct {
	FirstLogged  *string `json:"first_logged"`
	DaysWithData int     `json:"days_with_data"`
	DaysInRange  int     `json:"days_in_range"`
}

// AnalysisDay is one day's rollup. Food, WeightLb, and Exercise are each null
// when that domain had no logging on the day — never a misleading zero. The
// calorie/protein targets are configuration and are always present. This
// drives both the JSON Days array and the companion daily CSV.
type AnalysisDay struct {
	Date           string            `json:"date"`
	Food           *AnalysisDayFood  `json:"food"`
	CalorieTarget  int               `json:"calorie_target"`
	ProteinTargetG float64           `json:"protein_target_g"`
	SatFatTargetG  float64           `json:"sat_fat_target_g"`
	WeightLb       *float64          `json:"weight_lb"`
	Exercise       *AnalysisDayMoves `json:"exercise"`
}

// AnalysisDayFood is a day's as-eaten food totals and quality score. Present
// only when at least one meal was logged that day (else AnalysisDay.Food is
// null).
type AnalysisDayFood struct {
	Calories           int64   `json:"calories"`
	ProteinG           float64 `json:"protein_g"`
	CarbsG             float64 `json:"carbs_g"`
	FatG               float64 `json:"fat_g"`
	FiberG             float64 `json:"fiber_g"`
	SatFatG            float64 `json:"sat_fat_g"`
	SugarG             float64 `json:"sugar_g"`
	SodiumMg           int64   `json:"sodium_mg"`
	ProteinPctCalories float64 `json:"protein_pct_calories"`
	QualityScore       *int    `json:"quality_score"`
}

// AnalysisDayMoves is the per-day exercise summary — minutes by duration-based
// type plus a session count for strength (which carries no duration). ActiveMin
// is the sum of the four duration types. This is how exercise reaches the daily
// CSV; full per-session detail lives in AnalysisExport.Exercise.
type AnalysisDayMoves struct {
	CardioMin        int `json:"cardio_min"`
	StrengthSessions int `json:"strength_sessions"`
	YogaMin          int `json:"yoga_min"`
	MeditationMin    int `json:"meditation_min"`
	PTMin            int `json:"pt_min"`
	ActiveMin        int `json:"active_min"`
}

// AnalysisMealGroup is the analytical unit: one eating occasion, built by
// grouping raw entries on (date, slot) — the same grouping the day view
// renders. EntryCount and Descriptions expose the underlying logging
// granularity for debugging, but they describe how the meal was typed in, not
// how often Jim eats. Snacks legitimately repeat within a day, so snack
// entries are split into occasions by a time gap (OccasionIndex); main meals
// always collapse to one group per slot.
type AnalysisMealGroup struct {
	Date               string         `json:"date"`
	Slot               string         `json:"slot,omitempty"`
	OccasionIndex      int            `json:"occasion_index"`
	Calories           int64          `json:"calories"`
	ProteinG           float64        `json:"protein_g"`
	CarbsG             float64        `json:"carbs_g"`
	FatG               float64        `json:"fat_g"`
	FiberG             float64        `json:"fiber_g"`
	SatFatG            float64        `json:"sat_fat_g"`
	SugarG             float64        `json:"sugar_g"`
	SodiumMg           int64          `json:"sodium_mg"`
	ProteinPctCalories float64        `json:"protein_pct_calories"`
	QualityScore       *int           `json:"quality_score"`
	EntryCount         int            `json:"entry_count"`
	Descriptions       []string       `json:"descriptions"`
	Items              []AnalysisItem `json:"items"`
}

// AnalysisEntry is one raw logging entry (one DB meal row) in display units —
// the debug view under the grouped MealGroups. Do not treat these as eating
// occasions; use MealGroups for that.
type AnalysisEntry struct {
	Date        string         `json:"date"`
	Slot        string         `json:"slot,omitempty"`
	Description string         `json:"description,omitempty"`
	Calories    int64          `json:"calories"`
	ProteinG    float64        `json:"protein_g"`
	CarbsG      float64        `json:"carbs_g"`
	FatG        float64        `json:"fat_g"`
	FiberG      float64        `json:"fiber_g"`
	SatFatG     float64        `json:"sat_fat_g"`
	SugarG      float64        `json:"sugar_g"`
	SodiumMg    int64          `json:"sodium_mg"`
	Score       *int           `json:"score,omitempty"`
	Items       []AnalysisItem `json:"items"`
}

// AnalysisItem is one food item's as-eaten nutrition in display units.
type AnalysisItem struct {
	Name       string  `json:"name"`
	Brand      *string `json:"brand,omitempty"`
	Grams      *int    `json:"grams,omitempty"`
	Calories   int64   `json:"calories"`
	ProteinG   float64 `json:"protein_g"`
	CarbsG     float64 `json:"carbs_g"`
	FatG       float64 `json:"fat_g"`
	FiberG     float64 `json:"fiber_g"`
	SatFatG    float64 `json:"sat_fat_g"`
	SugarG     float64 `json:"sugar_g"`
	SodiumMg   int64   `json:"sodium_mg"`
	Tier       string  `json:"tier"`
	TierReason string  `json:"tier_reason,omitempty"`
	// Resolution provenance (item 3), so an analyst can see at a glance which
	// numbers are lab-analyzed FDC data and which are estimates. Omitted on
	// legacy items that predate the resolution layer.
	FDCDataType    *string `json:"fdc_data_type,omitempty"`
	ResolutionTier *int    `json:"resolution_tier,omitempty"`
	PortionSource  *string `json:"portion_source,omitempty"`
}

type AnalysisSession struct {
	Date        string         `json:"date"`
	Type        string         `json:"type"`
	Activity    *string        `json:"activity,omitempty"`
	Location    *string        `json:"location,omitempty"`
	Style       *string        `json:"style,omitempty"`
	DurationMin *int           `json:"duration_min,omitempty"`
	HRZones     map[string]int `json:"hr_zones,omitempty"`
	Note        string         `json:"note,omitempty"`
}

type AnalysisWeight struct {
	Date     string  `json:"date"`
	WeightLb float64 `json:"weight_lb"`
	Note     string  `json:"note,omitempty"`
}

// snackOccasionGap splits a day's snack entries into separate occasions: two
// snack entries more than this far apart in eaten_at are different snacking
// occasions, closer together they're one occasion logged as multiple entries.
// Main meals (breakfast/lunch/dinner) never split — they always collapse to
// one group per slot.
const snackOccasionGap = 120 * time.Minute

// slotRank orders meal groups within a day for deterministic output.
var slotRank = map[string]int{
	SlotBreakfast: 0, SlotLunch: 1, SlotDinner: 2, SlotSnack: 3,
}

func rankOf(slot string) int {
	if r, ok := slotRank[slot]; ok {
		return r
	}
	return 4 // unspecified / unknown slots sort last
}

// mgToG converts integer milligrams to grams rounded to one decimal. Display
// formatting only — stored data and derived math stay in integers (invariant).
func mgToG(mg int64) float64 { return math.Round(float64(mg)/100) / 10 }

// gramsToLb converts integer grams of body weight to pounds, one decimal — the
// same conversion the PWA uses (453.592 g/lb).
func gramsToLb(g int) float64 { return math.Round(float64(g)/453.592*10) / 10 }

// proteinPctCalories is the share of calories from protein (4 kcal/g), one
// decimal. Computed from integer milligrams and integer calories; 0 when there
// are no calories to divide into.
func proteinPctCalories(proteinMg, calories int64) float64 {
	if calories <= 0 {
		return 0
	}
	return math.Round(float64(proteinMg)*4/1000/float64(calories)*1000) / 10
}

// foodTotals accumulates as-eaten nutrition in the app's integer units while a
// set of items is converted to display-unit AnalysisItems. Shared by the day,
// meal-group, and per-entry aggregations so all three agree exactly.
type foodTotals struct {
	cal, protein, carbs, fat, fiber, satfat, sugar, sodium int64
}

// addItem folds one full-portion MealItem into the running totals (applying its
// portion fraction) and returns the item as an as-eaten AnalysisItem.
func (t *foodTotals) addItem(it MealItem) AnalysisItem {
	f := it.FractionPct
	cal := EatenValue(it.Calories, f)
	prot := EatenValue(it.ProteinMg, f)
	carb := EatenValue(it.CarbsMg, f)
	fat := EatenValue(it.FatMg, f)
	fiber := EatenValue(it.FiberMg, f)
	sat := EatenValue(it.SatFatMg, f)
	sugar := EatenValue(it.SugarMg, f)
	sodium := EatenValue(it.SodiumMg, f)
	t.cal += cal
	t.protein += prot
	t.carbs += carb
	t.fat += fat
	t.fiber += fiber
	t.satfat += sat
	t.sugar += sugar
	t.sodium += sodium

	var grams *int
	if it.Grams != nil {
		g := *it.Grams * f / 100
		grams = &g
	}
	return AnalysisItem{
		Name: it.Name, Brand: it.Brand, Grams: grams,
		Calories: cal, ProteinG: mgToG(prot), CarbsG: mgToG(carb), FatG: mgToG(fat),
		FiberG: mgToG(fiber), SatFatG: mgToG(sat), SugarG: mgToG(sugar), SodiumMg: sodium,
		Tier: it.Tier, TierReason: it.TierReason,
		FDCDataType: it.FDCDataType, ResolutionTier: it.ResolutionTier, PortionSource: it.PortionSource,
	}
}

// DataRange reports the earliest and latest logged day across all data.
func (s *Service) DataRange(ctx context.Context) (start, end string, ok bool, err error) {
	return s.store.DataRange(ctx)
}

// ExportAnalysis builds the LLM-ready export for the inclusive [start, end]
// day range. All nutrition math reuses EatenValue/DayScore so totals and
// scores match the rest of the app exactly.
func (s *Service) ExportAnalysis(ctx context.Context, start, end string) (*AnalysisExport, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	if start > end {
		return nil, fmt.Errorf("invalid: start must be on or before end")
	}

	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	meals, err := s.store.ListMealsRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	weights, err := s.store.ListWeights(ctx, start, end)
	if err != nil {
		return nil, err
	}
	sessions, err := s.store.ListExerciseRange(ctx, start, end)
	if err != nil {
		return nil, err
	}

	// Per-day accumulator. rawItems holds the day's MealItems so DayScore can
	// apply the fraction itself (matching the composite-score definition).
	type dayAgg struct {
		food        foodTotals
		rawItems    []MealItem
		hasFood     bool
		weightG     *int
		moves       AnalysisDayMoves
		hasExercise bool
	}
	days := map[string]*dayAgg{}
	dayOf := func(d string) *dayAgg {
		a := days[d]
		if a == nil {
			a = &dayAgg{}
			days[d] = a
		}
		return a
	}

	// Raw per-entry detail + per-day food accumulation.
	outEntries := make([]AnalysisEntry, 0, len(meals))
	for _, m := range meals {
		a := dayOf(m.Day)
		a.hasFood = true
		a.rawItems = append(a.rawItems, m.Items...)

		var tot foodTotals
		ae := AnalysisEntry{Date: m.Day, Description: m.Description, Items: make([]AnalysisItem, 0, len(m.Items))}
		if m.Slot != nil {
			ae.Slot = *m.Slot
		}
		for _, it := range m.Items {
			ai := tot.addItem(it)
			ae.Items = append(ae.Items, ai)
			a.food.addItem(it) // accumulate into the day total
		}
		ae.Calories = tot.cal
		ae.ProteinG = mgToG(tot.protein)
		ae.CarbsG = mgToG(tot.carbs)
		ae.FatG = mgToG(tot.fat)
		ae.FiberG = mgToG(tot.fiber)
		ae.SatFatG = mgToG(tot.satfat)
		ae.SugarG = mgToG(tot.sugar)
		ae.SodiumMg = tot.sodium
		if sc, ok := DayScore(m.Items); ok {
			ae.Score = &sc
		}
		outEntries = append(outEntries, ae)
	}

	mealGroups := buildMealGroups(meals)

	for _, wt := range weights {
		g := wt.WeightG
		dayOf(wt.Day).weightG = &g
	}

	outSessions := make([]AnalysisSession, 0, len(sessions))
	for _, e := range sessions {
		a := dayOf(e.Day)
		a.hasExercise = true
		dur := 0
		if e.DurationMin != nil {
			dur = *e.DurationMin
		}
		switch e.Type {
		case ExerciseCardio:
			a.moves.CardioMin += dur
		case ExerciseYoga:
			a.moves.YogaMin += dur
		case ExerciseMeditation:
			a.moves.MeditationMin += dur
		case ExercisePT:
			a.moves.PTMin += dur
		case ExerciseStrength:
			a.moves.StrengthSessions++
		}
		outSessions = append(outSessions, AnalysisSession{
			Date: e.Day, Type: e.Type, Activity: e.Activity, Location: e.Location,
			Style: e.Style, DurationMin: e.DurationMin, HRZones: e.HRZones, Note: e.Note,
		})
	}

	proteinTargetG := mgToG(settings.ProteinTargetMg)
	satFatTargetG := mgToG(settings.SatFatTargetMg)
	dates := make([]string, 0, len(days))
	for d := range days {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	outDays := make([]AnalysisDay, 0, len(dates))
	for _, d := range dates {
		a := days[d]
		ad := AnalysisDay{
			Date:          d,
			CalorieTarget: settings.CalorieTarget, ProteinTargetG: proteinTargetG, SatFatTargetG: satFatTargetG,
		}
		if a.hasFood {
			food := &AnalysisDayFood{
				Calories: a.food.cal,
				ProteinG: mgToG(a.food.protein), CarbsG: mgToG(a.food.carbs), FatG: mgToG(a.food.fat),
				FiberG: mgToG(a.food.fiber), SatFatG: mgToG(a.food.satfat), SugarG: mgToG(a.food.sugar),
				SodiumMg:           a.food.sodium,
				ProteinPctCalories: proteinPctCalories(a.food.protein, a.food.cal),
			}
			if sc, ok := DayScore(a.rawItems); ok {
				food.QualityScore = &sc
			}
			ad.Food = food
		}
		if a.weightG != nil {
			lb := gramsToLb(*a.weightG)
			ad.WeightLb = &lb
		}
		if a.hasExercise {
			moves := a.moves
			moves.ActiveMin = moves.CardioMin + moves.YogaMin + moves.MeditationMin + moves.PTMin
			ad.Exercise = &moves
		}
		outDays = append(outDays, ad)
	}

	outWeights := make([]AnalysisWeight, 0, len(weights))
	for _, wt := range weights {
		outWeights = append(outWeights, AnalysisWeight{Date: wt.Day, WeightLb: gramsToLb(wt.WeightG), Note: wt.Note})
	}

	// Deterministic ordering regardless of store implementation.
	sort.SliceStable(outEntries, func(i, j int) bool { return outEntries[i].Date < outEntries[j].Date })
	sort.SliceStable(outSessions, func(i, j int) bool { return outSessions[i].Date < outSessions[j].Date })
	sort.SliceStable(outWeights, func(i, j int) bool { return outWeights[i].Date < outWeights[j].Date })

	coverage := buildCoverage(start, end, meals, sessions, weights)

	return &AnalysisExport{
		Meta: AnalysisMeta{
			App:        "food",
			ExportedAt: time.Now().UTC(),
			RangeStart: start,
			RangeEnd:   end,
			Units: map[string]string{
				"calories":          "kcal",
				"macros":            "grams",
				"sodium":            "mg",
				"weight":            "lb",
				"exercise_duration": "minutes",
			},
			QualityScore:                   "0-100, calorie-weighted mean of per-item diet-quality tiers (higher is better); null on days with no calorie-bearing food",
			Coverage:                       coverage,
			MealGroupsAreTheAnalyticalUnit: "One object per (date, slot); snacks split into occasions by time gap. entry_count reflects logging granularity only and must never be interpreted as eating frequency.",
			Notes:                          "Nutrition values are as-eaten (portion fraction applied). Missing data is null, never 0 — see meta.coverage and analysis_warnings before averaging any domain. Exercise is tracked independently and never affects the calorie budget.",
		},
		AnalysisWarnings: buildWarnings(start, coverage),
		Days:             outDays,
		MealGroups:       mealGroups,
		Entries:          outEntries,
		Exercise:         outSessions,
		Weights:          outWeights,
	}, nil
}

// buildMealGroups collapses raw entries into eating occasions. Entries are
// grouped by (date, slot); every slot but snack yields exactly one group per
// day (occasion 0), while snack entries are split into occasions whenever
// their eaten_at times are more than snackOccasionGap apart. Output is ordered
// by date, slot rank, then occasion.
func buildMealGroups(meals []Meal) []AnalysisMealGroup {
	// Bucket by (date, slot). Slot is "" when unspecified.
	type key struct{ date, slot string }
	buckets := map[key][]Meal{}
	order := []key{}
	for _, m := range meals {
		slot := ""
		if m.Slot != nil {
			slot = *m.Slot
		}
		k := key{m.Day, slot}
		if _, seen := buckets[k]; !seen {
			order = append(order, k)
		}
		buckets[k] = append(buckets[k], m)
	}

	groups := make([]AnalysisMealGroup, 0, len(order))
	for _, k := range order {
		entries := buckets[k]
		// Deterministic within-bucket order: by eaten_at, then id.
		sort.SliceStable(entries, func(i, j int) bool {
			if !entries[i].EatenAt.Equal(entries[j].EatenAt) {
				return entries[i].EatenAt.Before(entries[j].EatenAt)
			}
			return entries[i].ID < entries[j].ID
		})

		// Split into occasions. Snacks break on a time gap; everything else is
		// a single occasion.
		occasions := [][]Meal{}
		if k.slot == SlotSnack {
			var cur []Meal
			for i, m := range entries {
				if i > 0 && m.EatenAt.Sub(entries[i-1].EatenAt) > snackOccasionGap {
					occasions = append(occasions, cur)
					cur = nil
				}
				cur = append(cur, m)
			}
			if len(cur) > 0 {
				occasions = append(occasions, cur)
			}
		} else {
			occasions = append(occasions, entries)
		}

		for idx, occ := range occasions {
			groups = append(groups, mealGroupOf(k.date, k.slot, idx, occ))
		}
	}

	sort.SliceStable(groups, func(i, j int) bool {
		if groups[i].Date != groups[j].Date {
			return groups[i].Date < groups[j].Date
		}
		if ri, rj := rankOf(groups[i].Slot), rankOf(groups[j].Slot); ri != rj {
			return ri < rj
		}
		return groups[i].OccasionIndex < groups[j].OccasionIndex
	})
	return groups
}

// mealGroupOf builds one meal group from the entries in a single occasion.
func mealGroupOf(date, slot string, occasion int, entries []Meal) AnalysisMealGroup {
	g := AnalysisMealGroup{
		Date: date, Slot: slot, OccasionIndex: occasion,
		EntryCount:   len(entries),
		Descriptions: []string{},
		Items:        []AnalysisItem{},
	}
	var tot foodTotals
	var raw []MealItem
	for _, m := range entries {
		if d := strings.TrimSpace(m.Description); d != "" {
			g.Descriptions = append(g.Descriptions, d)
		}
		for _, it := range m.Items {
			g.Items = append(g.Items, tot.addItem(it))
			raw = append(raw, it)
		}
	}
	g.Calories = tot.cal
	g.ProteinG = mgToG(tot.protein)
	g.CarbsG = mgToG(tot.carbs)
	g.FatG = mgToG(tot.fat)
	g.FiberG = mgToG(tot.fiber)
	g.SatFatG = mgToG(tot.satfat)
	g.SugarG = mgToG(tot.sugar)
	g.SodiumMg = tot.sodium
	g.ProteinPctCalories = proteinPctCalories(tot.protein, tot.cal)
	if sc, ok := DayScore(raw); ok {
		g.QualityScore = &sc
	}
	return g
}

// buildCoverage reports per-domain logging coverage over the requested range:
// when the domain's first in-range data appears and how many days carry it,
// against the total number of calendar days in range.
func buildCoverage(start, end string, meals []Meal, sessions []ExerciseSession, weights []Weight) map[string]AnalysisCoverage {
	total := daysInRange(start, end)

	foodDays := map[string]bool{}
	for _, m := range meals {
		foodDays[m.Day] = true
	}
	exDays := map[string]bool{}
	for _, e := range sessions {
		exDays[e.Day] = true
	}
	wtDays := map[string]bool{}
	for _, w := range weights {
		wtDays[w.Day] = true
	}

	return map[string]AnalysisCoverage{
		"food":     coverageOf(foodDays, total),
		"exercise": coverageOf(exDays, total),
		"weight":   coverageOf(wtDays, total),
	}
}

// coverageOf turns a set of days-with-data into a coverage record: earliest
// day (nil if none) and the count, against the range's total days.
func coverageOf(days map[string]bool, total int) AnalysisCoverage {
	cov := AnalysisCoverage{DaysWithData: len(days), DaysInRange: total}
	first := ""
	for d := range days {
		if first == "" || d < first {
			first = d
		}
	}
	if first != "" {
		cov.FirstLogged = &first
	}
	return cov
}

// buildWarnings auto-populates analysis_warnings so a consumer can't average
// over a partially-logged domain without the gap being spelled out. A domain
// with no data in range gets a "no data" warning; a domain whose logging began
// after the range start gets a "began mid-range, don't average" warning.
func buildWarnings(start string, coverage map[string]AnalysisCoverage) []string {
	var warnings []string

	food := coverage["food"]
	switch {
	case food.DaysWithData == 0:
		warnings = append(warnings, "No food entries in range. Nutrition conclusions cannot be drawn.")
	case food.FirstLogged != nil && *food.FirstLogged > start:
		warnings = append(warnings, fmt.Sprintf(
			"Food logging began %s. Do not compute food averages or consistency metrics across the full range.", *food.FirstLogged))
	}

	ex := coverage["exercise"]
	switch {
	case ex.DaysWithData == 0:
		warnings = append(warnings, "No exercise entries in range. Do not draw conclusions about training consistency.")
	case ex.FirstLogged != nil && *ex.FirstLogged > start:
		warnings = append(warnings, fmt.Sprintf(
			"Exercise logging began %s. Do not compute exercise averages or consistency metrics across the full range.", *ex.FirstLogged))
	}

	wt := coverage["weight"]
	if wt.DaysWithData == 0 {
		warnings = append(warnings, "No weight entries in range. Energy balance conclusions are input-side estimates only and cannot be validated against outcome.")
	}

	return warnings
}

// daysInRange counts calendar days in the inclusive [start, end] range. Both
// are validated YYYY-MM-DD dates by the time this is called.
func daysInRange(start, end string) int {
	s, err1 := time.Parse("2006-01-02", start)
	e, err2 := time.Parse("2006-01-02", end)
	if err1 != nil || err2 != nil {
		return 0
	}
	return int(e.Sub(s).Hours()/24) + 1
}
