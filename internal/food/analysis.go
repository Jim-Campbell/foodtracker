package food

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

// The analysis export (GET /api/export/analysis) is a reshaped, date-ranged
// view of the log built for feeding into an LLM (Claude) chat — distinct from
// the raw full-DB backup (GET /api/export). It differs from the backup in
// three ways that make it a better analytical substrate:
//
//   - Human-standard units: macros in grams, weight in pounds, calories in
//     kcal, sodium in milligrams — not the DB's integer-milligram storage.
//   - As-eaten values: the portion fraction is applied, so numbers reflect
//     what was actually consumed, not the full-portion figures on disk.
//   - No noise: ai_raw, the per-nutrient micros blob, source_ref, and
//     confidence are dropped; the quality tier + reason are kept.
//
// It leads with a denormalized per-day rollup (Days) — the compact, low-token
// part most analyses need — then per-meal item detail (Meals), full exercise
// sessions (Exercise), and weigh-ins (Weights).
type AnalysisExport struct {
	Meta     AnalysisMeta      `json:"meta"`
	Days     []AnalysisDay     `json:"days"`
	Meals    []AnalysisMeal    `json:"meals"`
	Exercise []AnalysisSession `json:"exercise"`
	Weights  []AnalysisWeight  `json:"weights"`
}

type AnalysisMeta struct {
	App          string            `json:"app"`
	ExportedAt   time.Time         `json:"exported_at"`
	RangeStart   string            `json:"range_start"`
	RangeEnd     string            `json:"range_end"`
	Units        map[string]string `json:"units"`
	QualityScore string            `json:"quality_score"`
	Notes        string            `json:"notes"`
}

// AnalysisDay is one day's as-eaten totals, quality score, weight, and an
// exercise summary. It drives both the JSON Days array and the companion
// daily CSV, so every field here is a flat scalar.
type AnalysisDay struct {
	Date           string           `json:"date"`
	Calories       int64            `json:"calories"`
	ProteinG       float64          `json:"protein_g"`
	CarbsG         float64          `json:"carbs_g"`
	FatG           float64          `json:"fat_g"`
	FiberG         float64          `json:"fiber_g"`
	SatFatG        float64          `json:"sat_fat_g"`
	SugarG         float64          `json:"sugar_g"`
	SodiumMg       int64            `json:"sodium_mg"`
	QualityScore   *int             `json:"quality_score"`
	CalorieTarget  int              `json:"calorie_target"`
	ProteinTargetG float64          `json:"protein_target_g"`
	WeightLb       *float64         `json:"weight_lb"`
	Exercise       AnalysisDayMoves `json:"exercise"`
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

type AnalysisMeal struct {
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

// mgToG converts integer milligrams to grams rounded to one decimal. Display
// formatting only — stored data and derived math stay in integers (invariant).
func mgToG(mg int64) float64 { return math.Round(float64(mg)/100) / 10 }

// gramsToLb converts integer grams of body weight to pounds, one decimal — the
// same conversion the PWA uses (453.592 g/lb).
func gramsToLb(g int) float64 { return math.Round(float64(g)/453.592*10) / 10 }

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

	// Per-day accumulator. items holds the day's raw MealItems so DayScore can
	// apply the fraction itself (matching the composite-score definition).
	type dayAgg struct {
		cal, protein, carbs, fat, fiber, satfat, sugar, sodium int64
		items                                                  []MealItem
		weightG                                                *int
		moves                                                  AnalysisDayMoves
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

	outMeals := make([]AnalysisMeal, 0, len(meals))
	for _, m := range meals {
		a := dayOf(m.Day)
		am := AnalysisMeal{Date: m.Day, Description: m.Description, Items: make([]AnalysisItem, 0, len(m.Items))}
		if m.Slot != nil {
			am.Slot = *m.Slot
		}
		var mCal, mProt, mCarb, mFat, mFiber, mSat, mSugar, mSodium int64
		for _, it := range m.Items {
			f := it.FractionPct
			cal := EatenValue(it.Calories, f)
			prot := EatenValue(it.ProteinMg, f)
			carb := EatenValue(it.CarbsMg, f)
			fat := EatenValue(it.FatMg, f)
			fiber := EatenValue(it.FiberMg, f)
			sat := EatenValue(it.SatFatMg, f)
			sugar := EatenValue(it.SugarMg, f)
			sodium := EatenValue(it.SodiumMg, f)
			var grams *int
			if it.Grams != nil {
				g := *it.Grams * f / 100
				grams = &g
			}
			am.Items = append(am.Items, AnalysisItem{
				Name: it.Name, Brand: it.Brand, Grams: grams,
				Calories: cal, ProteinG: mgToG(prot), CarbsG: mgToG(carb), FatG: mgToG(fat),
				FiberG: mgToG(fiber), SatFatG: mgToG(sat), SugarG: mgToG(sugar), SodiumMg: sodium,
				Tier: it.Tier, TierReason: it.TierReason,
			})
			mCal += cal
			mProt += prot
			mCarb += carb
			mFat += fat
			mFiber += fiber
			mSat += sat
			mSugar += sugar
			mSodium += sodium
		}
		am.Calories = mCal
		am.ProteinG = mgToG(mProt)
		am.CarbsG = mgToG(mCarb)
		am.FatG = mgToG(mFat)
		am.FiberG = mgToG(mFiber)
		am.SatFatG = mgToG(mSat)
		am.SugarG = mgToG(mSugar)
		am.SodiumMg = mSodium
		if sc, ok := DayScore(m.Items); ok {
			am.Score = &sc
		}
		outMeals = append(outMeals, am)

		a.cal += mCal
		a.protein += mProt
		a.carbs += mCarb
		a.fat += mFat
		a.fiber += mFiber
		a.satfat += mSat
		a.sugar += mSugar
		a.sodium += mSodium
		a.items = append(a.items, m.Items...)
	}

	for _, wt := range weights {
		g := wt.WeightG
		dayOf(wt.Day).weightG = &g
	}

	outSessions := make([]AnalysisSession, 0, len(sessions))
	for _, e := range sessions {
		a := dayOf(e.Day)
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
	dates := make([]string, 0, len(days))
	for d := range days {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	outDays := make([]AnalysisDay, 0, len(dates))
	for _, d := range dates {
		a := days[d]
		ad := AnalysisDay{
			Date: d, Calories: a.cal,
			ProteinG: mgToG(a.protein), CarbsG: mgToG(a.carbs), FatG: mgToG(a.fat),
			FiberG: mgToG(a.fiber), SatFatG: mgToG(a.satfat), SugarG: mgToG(a.sugar), SodiumMg: a.sodium,
			CalorieTarget: settings.CalorieTarget, ProteinTargetG: proteinTargetG,
			Exercise: a.moves,
		}
		ad.Exercise.ActiveMin = a.moves.CardioMin + a.moves.YogaMin + a.moves.MeditationMin + a.moves.PTMin
		if sc, ok := DayScore(a.items); ok {
			ad.QualityScore = &sc
		}
		if a.weightG != nil {
			lb := gramsToLb(*a.weightG)
			ad.WeightLb = &lb
		}
		outDays = append(outDays, ad)
	}

	outWeights := make([]AnalysisWeight, 0, len(weights))
	for _, wt := range weights {
		outWeights = append(outWeights, AnalysisWeight{Date: wt.Day, WeightLb: gramsToLb(wt.WeightG), Note: wt.Note})
	}

	// Deterministic ordering regardless of store implementation.
	sort.SliceStable(outMeals, func(i, j int) bool { return outMeals[i].Date < outMeals[j].Date })
	sort.SliceStable(outSessions, func(i, j int) bool { return outSessions[i].Date < outSessions[j].Date })
	sort.SliceStable(outWeights, func(i, j int) bool { return outWeights[i].Date < outWeights[j].Date })

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
			QualityScore: "0-100, calorie-weighted mean of per-item diet-quality tiers (higher is better); null on days with no calorie-bearing food",
			Notes:        "Nutrition values are as-eaten (portion fraction applied). Exercise is tracked independently and never affects the calorie budget.",
		},
		Days:     outDays,
		Meals:    outMeals,
		Exercise: outSessions,
		Weights:  outWeights,
	}, nil
}
