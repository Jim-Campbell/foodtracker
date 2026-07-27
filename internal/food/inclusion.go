package food

import (
	"sort"
	"time"
)

// Inclusion score (inclusion phase 1): a second, independent score alongside
// the calorie-weighted composition score (score.go). Composition asks "of the
// calories I ate, what tier were they?" Inclusion asks "did I eat the handful
// of foods with the strongest evidence behind them?" Calorie-trivial foods
// (kale, berries) can never move a calorie-weighted mean, which is why this
// exists as its own construct rather than a weight inside DayScore.
//
// The two scores are never merged, averaged, or combined into one number.

// Component ids: fixed at five for launch (inclusion-spec-20260726.md §2).
// Deliberately excludes nuts/seeds (permanently-satisfied target, pure noise)
// and defers EVOO/fermented/whole-grains to phase 2.
const (
	ComponentLeafyGreens = "leafy_greens"
	ComponentBerries     = "berries"
	ComponentLegumes     = "legumes"
	ComponentFattyFish   = "fatty_fish"
	ComponentCruciferous = "cruciferous"
)

// ComponentDef is one component's catalog entry: id, display, weekly target,
// and the default reference serving used to pre-fill a new tag.
type ComponentDef struct {
	ID                     string `json:"id"`
	Label                  string `json:"label"`
	Emoji                  string `json:"emoji"`
	ServingText            string `json:"serving_text"`
	Target                 int    `json:"target"`                    // per rolling 7-day window
	DefaultGramsPerServing int    `json:"default_grams_per_serving"` // pre-fill for a new tag
}

// ComponentCatalog is the single source of truth for the five components, in
// display order. Targets live here as Go constants for now; a settings
// override lands in inclusion phase 5.
var ComponentCatalog = []ComponentDef{
	{ID: ComponentLeafyGreens, Label: "Leafy greens", Emoji: "🥬", ServingText: "1 cup raw / ½ cup cooked", Target: 7, DefaultGramsPerServing: 30},
	{ID: ComponentBerries, Label: "Berries", Emoji: "🫐", ServingText: "½ cup", Target: 5, DefaultGramsPerServing: 75},
	{ID: ComponentLegumes, Label: "Legumes", Emoji: "🫘", ServingText: "½ cup cooked", Target: 5, DefaultGramsPerServing: 90},
	{ID: ComponentFattyFish, Label: "Fatty fish", Emoji: "🐟", ServingText: "3–4 oz", Target: 3, DefaultGramsPerServing: 100},
	{ID: ComponentCruciferous, Label: "Cruciferous", Emoji: "🥦", ServingText: "1 cup raw / ½ cup cooked", Target: 4, DefaultGramsPerServing: 85},
}

var validComponentIDs = func() map[string]bool {
	m := make(map[string]bool, len(ComponentCatalog))
	for _, c := range ComponentCatalog {
		m[c.ID] = true
	}
	return m
}()

// Tag sources: 'ai' when a parse-time confirmation populates the tag (phase
// 2), 'user' when set explicitly via the API.
const (
	TagSourceAI   = "ai"
	TagSourceUser = "user"
)

// MaxServingsX100PerMeal is the per-meal cap on a component's contribution:
// 2.00 servings, expressed in hundredths. The construct is exposure frequency
// over time, not volume, so one enormous salad must not satisfy the weekly
// greens target. Spec §10 open decision 1 — keep this a one-line change.
const MaxServingsX100PerMeal = 200

// ComponentTag maps a logged food (by normalized name, with an optional
// fdc_id secondary key) to a component it counts toward, and the grams that
// make up one serving of it. Multi-tag is expected: kale is both
// leafy_greens and cruciferous.
type ComponentTag struct {
	ID              int64     `json:"id"`
	NormalizedName  string    `json:"normalized_name"`
	FDCID           *int64    `json:"fdc_id,omitempty"`
	ComponentID     string    `json:"component_id"`
	GramsPerServing int       `json:"grams_per_serving"`
	TagSource       string    `json:"tag_source"`
	CreatedAt       time.Time `json:"created_at,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// ComponentProgress is one component's standing within a window (rolling or
// calendar-week).
type ComponentProgress struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Emoji         string `json:"emoji"`
	Target        int    `json:"target"`
	ServingsX100  int64  `json:"servings_x100"`
	Whole         int    `json:"whole"`          // whole servings logged, for the dot row
	ExpiringWhole int    `json:"expiring_whole"` // whole servings aging out at the start of tomorrow (rolling window only)
	ProgressPct   int    `json:"progress_pct"`   // min(servings/target, 1.0) * 100
	Met           bool   `json:"met"`
}

// InclusionWindow is the inclusion score over some [Start, End] day range —
// the rolling last-7-days window on Today, or one completed calendar week in
// Trends. UnmeasuredFoods counts logged foods that couldn't contribute
// because they carry no grams, so the miss is visible rather than silent.
type InclusionWindow struct {
	Start           string              `json:"start"`
	End             string              `json:"end"`
	Score           int                 `json:"score"`
	Components      []ComponentProgress `json:"components"`
	UnmeasuredFoods int                 `json:"unmeasured_foods"`
	// Nudge candidates (inclusion phase 5): populated only for the rolling
	// Today window (Service.InclusionWindow), never for the Trends weekly
	// retrospective. Both are derived from Components alone -- see
	// SupplyNeeds/DecisionCandidate for the honesty constraint this encodes.
	SupplyNeeds       []ComponentProgress `json:"supply_needs,omitempty"`
	DecisionCandidate *ComponentProgress  `json:"decision_candidate,omitempty"`
}

// InclusionWeek is one completed Mon-Sun calendar week's retrospective (spec
// §8 — the only place calendar weeks apply to food).
type InclusionWeek struct {
	WeekStart     string              `json:"week_start"`
	ComponentsHit int                 `json:"components_hit"`
	Components    []ComponentProgress `json:"components"`
}

// TagCandidate is one row in the Settings → "Tag foods" backfill screen
// (inclusion phase 2 §4): a distinct logged food with its current tags and a
// times-logged sort key, so the highest-leverage untagged foods surface
// first.
type TagCandidate struct {
	NormalizedName string         `json:"normalized_name"`
	DisplayName    string         `json:"display_name"`
	FDCID          *int64         `json:"fdc_id,omitempty"`
	TimesLogged    int            `json:"times_logged"`
	Tags           []ComponentTag `json:"tags"`
}

// componentTagIndex resolves a logged food to its tags: fdc_id exact hit ->
// normalized_name hit -> untagged.
type componentTagIndex struct {
	byFDCID map[int64][]ComponentTag
	byName  map[string][]ComponentTag
}

func buildTagIndex(tags []ComponentTag) componentTagIndex {
	idx := componentTagIndex{byFDCID: map[int64][]ComponentTag{}, byName: map[string][]ComponentTag{}}
	for _, t := range tags {
		if t.FDCID != nil {
			idx.byFDCID[*t.FDCID] = append(idx.byFDCID[*t.FDCID], t)
		}
		idx.byName[t.NormalizedName] = append(idx.byName[t.NormalizedName], t)
	}
	return idx
}

func (idx componentTagIndex) tagsFor(it MealItem) []ComponentTag {
	if it.FDCID != nil {
		if tags, ok := idx.byFDCID[*it.FDCID]; ok {
			return tags
		}
	}
	if tags, ok := idx.byName[NormalizeFoodName(it.Name)]; ok {
		return tags
	}
	return nil
}

// ComputeInclusion is the pure counter over a set of meals, the tag table,
// and explicit window bounds — no DB, fully unit-testable. start/end are
// YYYY-MM-DD, inclusive; meals outside the range are ignored so callers can
// pass a superset (e.g. one query spanning several weeks). targets overrides
// ComponentCatalog's default target per component id (Settings.ComponentTargets,
// inclusion phase 5 §"Tunable targets"); a missing or non-positive entry falls
// back to the catalog default, so passing nil reproduces the phase-1 behavior
// exactly.
func ComputeInclusion(meals []Meal, tags []ComponentTag, start, end string, targets map[string]int) InclusionWindow {
	idx := buildTagIndex(tags)
	counts := make(map[string]int64, len(ComponentCatalog))
	expiring := make(map[string]int64, len(ComponentCatalog))
	var unmeasured int

	for _, m := range meals {
		if m.Day < start || m.Day > end {
			continue
		}
		perMeal := map[string]int64{}
		for _, it := range m.Items {
			if it.Grams == nil || *it.Grams <= 0 {
				unmeasured++
				continue
			}
			itemTags := idx.tagsFor(it)
			if len(itemTags) == 0 {
				continue
			}
			gramsEaten := int64(*it.Grams) * int64(it.FractionPct) / 100
			for _, tag := range itemTags {
				if tag.GramsPerServing <= 0 {
					continue
				}
				perMeal[tag.ComponentID] += gramsEaten * 100 / int64(tag.GramsPerServing)
			}
		}
		for compID, sum := range perMeal {
			capped := sum
			if capped > MaxServingsX100PerMeal {
				capped = MaxServingsX100PerMeal
			}
			counts[compID] += capped
		}
		// end-6 is the day that ages out at the start of tomorrow; only
		// meaningful for a rolling 7-day window (start == end-6 by
		// construction there), harmless to compute for a calendar week too.
		expDay := dateAddDays(end, -6)
		if m.Day == expDay {
			for compID, sum := range perMeal {
				capped := sum
				if capped > MaxServingsX100PerMeal {
					capped = MaxServingsX100PerMeal
				}
				expiring[compID] += capped
			}
		}
	}

	components := make([]ComponentProgress, 0, len(ComponentCatalog))
	var scoreSum int
	for _, def := range ComponentCatalog {
		target := def.Target
		if t, ok := targets[def.ID]; ok && t > 0 {
			target = t
		}
		x100 := counts[def.ID]
		progressPct := int(x100 / int64(target))
		if progressPct > 100 {
			progressPct = 100
		}
		components = append(components, ComponentProgress{
			ID:            def.ID,
			Label:         def.Label,
			Emoji:         def.Emoji,
			Target:        target,
			ServingsX100:  x100,
			Whole:         int(x100 / 100),
			ExpiringWhole: int(expiring[def.ID] / 100),
			ProgressPct:   progressPct,
			Met:           x100 >= int64(target)*100,
		})
		scoreSum += progressPct
	}

	return InclusionWindow{
		Start:           start,
		End:             end,
		Score:           scoreSum / len(ComponentCatalog),
		Components:      components,
		UnmeasuredFoods: unmeasured,
	}
}

// SanitizeComponentTargets validates a Settings.ComponentTargets override map
// before it's persisted (inclusion phase 5 §"Tunable targets"): an unknown
// component id is dropped, same as SanitizeItemComponents — the model or a
// hand-edited request is never trusted with this invariant — and a target
// outside 1..21 is dropped rather than clamped, since an out-of-range weekly
// target reads as a typo, not an intentional edge case. Returns nil (not an
// empty map) when nothing survives.
func SanitizeComponentTargets(overrides map[string]int) map[string]int {
	if len(overrides) == 0 {
		return nil
	}
	out := make(map[string]int, len(overrides))
	for id, t := range overrides {
		if !validComponentIDs[id] || t < 1 || t > 21 {
			continue
		}
		out[id] = t
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// relativeGap is (target - servings) / target computed from the hundredths
// fields directly, never from the already-rounded ProgressPct — the rounding
// there is fine for display but would distort ranking (e.g. 6/7 rounds to a
// different gap than the true 1/7).
func relativeGap(c ComponentProgress) float64 {
	targetX100 := float64(c.Target) * 100
	return (targetX100 - float64(c.ServingsX100)) / targetX100
}

// SupplyNeeds ranks the components that will miss target if nothing changes
// (inclusion phase 5, tier 1 — the supply nudge): components with
// ProgressPct < 100, ranked by relative gap descending, top 3. Spec §7 asks
// for what's "projected to miss over the next 7 days"; since every serving
// counted today ages out somewhere within the coming 7-day window regardless
// of future action, a literal projection collapses to the current gap, so
// this computes that gap directly rather than modeling a projection that
// would produce the identical number.
//
// Takes only an InclusionWindow — never the composition score, calories, or
// macros (inclusion-spec-20260726.md §7 honesty constraint: a reviewer can
// confirm this from the signature alone).
func SupplyNeeds(w InclusionWindow) []ComponentProgress {
	var candidates []ComponentProgress
	for _, c := range w.Components {
		if c.Met {
			continue
		}
		candidates = append(candidates, c)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return relativeGap(candidates[i]) > relativeGap(candidates[j])
	})
	if len(candidates) > 3 {
		candidates = candidates[:3]
	}
	return candidates
}

// DecisionCandidate selects the single best decision-nudge candidate
// (inclusion phase 5, tier 2): among components not at target, the largest
// relative gap, tying toward the one with a serving aging out within 24h
// (ExpiringWhole > 0) — that's the one where action today prevents a loss.
// Returns nil when every component is at target.
//
// Takes only an InclusionWindow — see SupplyNeeds; same honesty constraint.
func DecisionCandidate(w InclusionWindow) *ComponentProgress {
	var best *ComponentProgress
	var bestGap float64
	for i := range w.Components {
		c := &w.Components[i]
		if c.Met {
			continue
		}
		gap := relativeGap(*c)
		switch {
		case best == nil:
			best, bestGap = c, gap
		case gap > bestGap:
			best, bestGap = c, gap
		case gap == bestGap && c.ExpiringWhole > 0 && best.ExpiringWhole == 0:
			best, bestGap = c, gap
		}
	}
	return best
}

// SanitizeItemComponents cleans a draft/saved item's proposed component tags
// before they're shown, saved, or accreted (inclusion phase 2): an unknown
// component_id is dropped rather than 500-ing (the model, or a hand-edited
// request, is never trusted with this invariant), grams_per_serving clamps to
// the same 1..2000 range the DB check constraint enforces, and a duplicate
// component_id collapses to its first occurrence. Returns nil (not an empty
// slice) when nothing survives, so it round-trips cleanly through
// omitempty.
func SanitizeItemComponents(comps []ItemComponent) []ItemComponent {
	if len(comps) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(comps))
	out := make([]ItemComponent, 0, len(comps))
	for _, c := range comps {
		if !validComponentIDs[c.ComponentID] || seen[c.ComponentID] {
			continue
		}
		seen[c.ComponentID] = true
		switch {
		case c.GramsPerServing < 1:
			c.GramsPerServing = 1
		case c.GramsPerServing > 2000:
			c.GramsPerServing = 2000
		}
		out = append(out, c)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dateAddDays adds n days (n may be negative) to a YYYY-MM-DD date string.
// Invalid input returns "" so a bad end never matches a real meal day.
func dateAddDays(day string, n int) string {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return ""
	}
	return t.AddDate(0, 0, n).Format("2006-01-02")
}

// mondayOnOrAfter returns the Monday of t's week if t is already a Monday,
// else the following Monday — the first candidate week-start for
// InclusionWeeks, so only whole weeks inside [start, end] are considered.
func mondayOnOrAfter(t time.Time) time.Time {
	offset := (int(t.Weekday()) + 6) % 7 // Monday=0 .. Sunday=6
	monday := t.AddDate(0, 0, -offset)
	if monday.Before(t) {
		monday = monday.AddDate(0, 0, 7)
	}
	return monday
}
