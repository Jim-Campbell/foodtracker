package food

import (
	"encoding/json"
	"time"
)

// Quality tiers. neutral = not addressed by docs/diet-framework.md.
const (
	TierHardYes = "hard_yes"
	TierSoftYes = "soft_yes"
	TierNeutral = "neutral"
	TierSoftNo  = "soft_no"
	TierHardNo  = "hard_no"
)

// TierValue is the point value used by the calorie-weighted day score.
var TierValue = map[string]int{
	TierHardYes: 100,
	TierSoftYes: 75,
	TierNeutral: 50,
	TierSoftNo:  25,
	TierHardNo:  0,
}

const (
	SourceUSDA   = "usda"
	SourceOFF    = "off"
	SourceAI     = "ai"
	SourceLabel  = "label"
	SourceManual = "manual"
	SourceWeb    = "web" // published nutrition found via web search (source_ref = URL)
)

const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

// PortionSource records how an item's portion weight was arrived at — the
// dominant error source once nutrition lookup is authoritative (item 3).
const (
	PortionWeighed     = "weighed"      // put on a kitchen scale
	PortionPackageUnit = "package_unit" // a whole package / labeled serving unit
	PortionEstimated   = "estimated"    // eyeballed
)

// TierSource records whether a quality tier came from an explicit entry in the
// diet framework (table) or from its Default Rule decision cascade for an
// unlisted food (cascade) — so a genuine "neutral" is distinguishable from a
// lookup miss (item 3, framework v3).
const (
	TierSourceTable   = "table"
	TierSourceCascade = "cascade"
)

// ResolutionTier is the FDC cascade tier that answered (item 3): 1 = barcode /
// Branded / label, 2 = Foundation or SR Legacy, 3 = Survey (FNDDS) or web,
// 4 = LLM estimate (last resort).
const (
	ResolutionBrandedLabel = 1
	ResolutionFoundationSR = 2
	ResolutionSurveyWeb    = 3
	ResolutionLLMEstimate  = 4
)

const (
	InputText    = "text"
	InputVoice   = "voice"
	InputPhoto   = "photo"
	InputBarcode = "barcode"
	InputManual  = "manual"
)

const (
	SlotBreakfast = "breakfast"
	SlotLunch     = "lunch"
	SlotDinner    = "dinner"
	SlotSnack     = "snack"
)

// Exercise types and input kinds. Exercise never touches the calorie budget.
const (
	ExerciseCardio     = "cardio"
	ExerciseStrength   = "strength"
	ExerciseYoga       = "yoga"
	ExerciseMeditation = "meditation"
	ExercisePT         = "pt"
)

const (
	ExerciseInputTap    = "tap"
	ExerciseInputText   = "text"
	ExerciseInputVoice  = "voice"
	ExerciseInputImport = "import"
)

var validExerciseTypes = map[string]bool{
	ExerciseCardio: true, ExerciseStrength: true, ExerciseYoga: true, ExerciseMeditation: true, ExercisePT: true,
}
var validExerciseInputKinds = map[string]bool{
	ExerciseInputTap: true, ExerciseInputText: true, ExerciseInputVoice: true, ExerciseInputImport: true,
}

var validTiers = map[string]bool{
	TierHardYes: true, TierSoftYes: true, TierNeutral: true, TierSoftNo: true, TierHardNo: true,
}
var validSources = map[string]bool{
	SourceUSDA: true, SourceOFF: true, SourceAI: true, SourceLabel: true, SourceManual: true, SourceWeb: true,
}
var validConfidences = map[string]bool{
	ConfidenceHigh: true, ConfidenceMedium: true, ConfidenceLow: true,
}
var validPortionSources = map[string]bool{
	PortionWeighed: true, PortionPackageUnit: true, PortionEstimated: true,
}
var validTierSources = map[string]bool{
	TierSourceTable: true, TierSourceCascade: true,
}
var validInputKinds = map[string]bool{
	InputText: true, InputVoice: true, InputPhoto: true, InputBarcode: true, InputManual: true,
}
var validSlots = map[string]bool{
	SlotBreakfast: true, SlotLunch: true, SlotDinner: true, SlotSnack: true,
}

// Meal is one logged eating event. Day is a user-chosen date, independent of
// EatenAt, so a late-night snack can be filed under the previous day.
type Meal struct {
	ID          int64           `json:"id"`
	Day         string          `json:"day"` // YYYY-MM-DD
	EatenAt     time.Time       `json:"eaten_at"`
	Slot        *string         `json:"slot,omitempty"`
	Description string          `json:"description"`
	InputKind   string          `json:"input_kind"`
	PhotoKey    *string         `json:"photo_key,omitempty"`
	PhotoURL    *string         `json:"photo_url,omitempty"`
	AIModel     *string         `json:"ai_model,omitempty"`
	AIRaw       json.RawMessage `json:"ai_raw,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at"`
	Items       []MealItem      `json:"items"`
}

// MealItem stores full-portion nutrition; as-eaten = value * FractionPct / 100.
// Calories are integer kcal; macro/micro amounts are integer milligrams.
type MealItem struct {
	ID          int64           `json:"id,omitempty"`
	MealID      int64           `json:"meal_id,omitempty"`
	Position    int             `json:"position"`
	Name        string          `json:"name"`
	Brand       *string         `json:"brand,omitempty"`
	Quantity    string          `json:"quantity"`
	Grams       *int            `json:"grams,omitempty"`
	FractionPct int             `json:"fraction_pct"`
	Calories    int64           `json:"calories"`
	ProteinMg   int64           `json:"protein_mg"`
	CarbsMg     int64           `json:"carbs_mg"`
	FatMg       int64           `json:"fat_mg"`
	FiberMg     int64           `json:"fiber_mg"`
	SatFatMg    int64           `json:"sat_fat_mg"`
	SugarMg     int64           `json:"sugar_mg"`
	SodiumMg    int64           `json:"sodium_mg"`
	Micros      json.RawMessage `json:"micros,omitempty"`
	Tier        string          `json:"tier"`
	TierReason  string          `json:"tier_reason"`
	Source      string          `json:"source"`
	SourceRef   *string         `json:"source_ref,omitempty"`
	Confidence  string          `json:"confidence"`
	// Resolution provenance (item 3). All nil on legacy rows and on manual
	// entries; populated by the parser's resolution cascade.
	FDCID          *int64  `json:"fdc_id,omitempty"`
	FDCDataType    *string `json:"fdc_data_type,omitempty"`   // Branded | Foundation | SR Legacy | Survey (FNDDS)
	ResolutionTier *int    `json:"resolution_tier,omitempty"` // 1..4, see ResolutionTier consts
	PortionSource  *string `json:"portion_source,omitempty"`  // weighed | package_unit | estimated
	TierSource     *string `json:"tier_source,omitempty"`     // table | cascade
}

// Weight is one day's weigh-in; a second entry for the same day replaces it.
type Weight struct {
	Day     string `json:"day"` // YYYY-MM-DD
	WeightG int    `json:"weight_g"`
	Note    string `json:"note"`
}

// Settings is the single-row (id=1) app configuration.
type Settings struct {
	CalorieTarget        int       `json:"calorie_target"`
	ProteinTargetMg      int64     `json:"protein_target_mg"`
	SatFatTargetMg       int64     `json:"sat_fat_target_mg"`
	WeightTargetG        *int      `json:"weight_target_g,omitempty"`
	CardioWeeklyTarget   int       `json:"cardio_weekly_target"`
	StrengthWeeklyTarget int       `json:"strength_weekly_target"`
	YogaWeeklyTarget     int       `json:"yoga_weekly_target"`
	MeditationWeeklyDays int       `json:"meditation_weekly_days"`
	PTWeeklyDays         int       `json:"pt_weekly_days"`
	UpdatedAt            time.Time `json:"updated_at,omitempty"`
}

// ExerciseSession is one logged workout/practice. Day is a user-chosen date,
// independent of PerformedAt, exactly like Meal.Day. Fully separate from the
// food/calorie domain — never credits or debits the calorie budget. Multiple
// sessions per day per type are normal (yoga twice, a hike after a swim);
// saves always append, never upsert.
type ExerciseSession struct {
	ID          int64     `json:"id"`
	Day         string    `json:"day"` // YYYY-MM-DD
	PerformedAt time.Time `json:"performed_at"`
	Type        string    `json:"type"`
	Activity    *string   `json:"activity,omitempty"`
	Location    *string   `json:"location,omitempty"`
	Style       *string   `json:"style,omitempty"`
	DurationMin *int      `json:"duration_min,omitempty"`
	// HRZones maps heart-rate zone ("1".."5") to integer minutes spent there.
	// Cardio only, optional, independent of DurationMin. Nil/empty when unused.
	// Garmin-import-ready: a future import populates this same shape.
	HRZones   map[string]int  `json:"hr_zones,omitempty"`
	Note      string          `json:"note"`
	InputKind string          `json:"input_kind"`
	AIRaw     json.RawMessage `json:"ai_raw,omitempty"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// DaySummary is the as-eaten totals and quality score for one day, plus the
// day's meals. Score is nil when the day has no calorie-bearing items.
type DaySummary struct {
	Day                string      `json:"day"`
	Calories           int64       `json:"calories"`
	ProteinMg          int64       `json:"protein_mg"`
	CarbsMg            int64       `json:"carbs_mg"`
	FatMg              int64       `json:"fat_mg"`
	FiberMg            int64       `json:"fiber_mg"`
	SatFatMg           int64       `json:"sat_fat_mg"`
	Score              *int        `json:"score"`
	CalorieTarget      int         `json:"calorie_target"`
	ProteinTargetMg    int64       `json:"protein_target_mg"`
	SatFatTargetMg     int64       `json:"sat_fat_target_mg"`
	CaloriesRemaining  int64       `json:"calories_remaining"`
	ProteinRemainingMg int64       `json:"protein_remaining_mg"`
	Anomaly            *DayAnomaly `json:"anomaly,omitempty"`
	Meals              []Meal      `json:"meals"`
}

// DayAnomaly is the outlier attribution for a day (item 6): it exists only when
// the day's quality score fell meaningfully below its own trailing baseline, or
// the day went over the saturated-fat ceiling. Nil on normal days, so the UI
// shows nothing. Headline is the one-line summary; Reasons are the 2-3 specific
// foods that drove it. This is an anomaly detector, not a daily grade.
type DayAnomaly struct {
	Headline string   `json:"headline"`
	Reasons  []string `json:"reasons"`
}

// RangeDay is one day's as-eaten totals for the trends view.
type RangeDay struct {
	Day        string `json:"day"`
	Calories   int64  `json:"calories"`
	ProteinMg  int64  `json:"protein_mg"`
	CarbsMg    int64  `json:"carbs_mg"`
	FatMg      int64  `json:"fat_mg"`
	Score      *int   `json:"score"`
	OverTarget bool   `json:"over_target"`
}

// Parse result kinds: the one log bar understands both food and exercise
// (phase E4), and the parse result carries a discriminator so the PWA knows
// which confirm flow to show.
const (
	ParseKindMeal     = "meal"
	ParseKindExercise = "exercise"
)

// ParseResult is the draft returned by the AI parse endpoints (phase 3+).
// Items use the same shape as MealItem so the client can edit the draft and
// POST it to /api/meals unchanged. Kind defaults to "meal" so existing
// behavior is unchanged; Exercise is populated instead of Items when Kind is
// "exercise" (phase E4 natural-language exercise logging).
type ParseResult struct {
	Kind     string            `json:"kind"`
	Items    []MealItem        `json:"items"`
	Exercise []ExerciseSession `json:"exercise,omitempty"`
	Notes    string            `json:"notes"`
	AIModel  string            `json:"ai_model"`
	AIRaw    json.RawMessage   `json:"ai_raw,omitempty"`
}

// Favorite is a reusable meal template: a named snapshot of items that can
// be re-logged with one tap. Items are a snapshot, not references — editing
// or deleting the original meal never mutates a favorite.
type Favorite struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Items     []MealItem `json:"items"`
	CreatedAt time.Time  `json:"created_at"`
}

// ExportDoc is the full-database backup returned by GET /api/export — the
// backup story for Render's ephemeral disk. Meals are ordered by day.
type ExportDoc struct {
	ExportedAt time.Time         `json:"exported_at"`
	Settings   Settings          `json:"settings"`
	Weights    []Weight          `json:"weights"`
	Meals      []Meal            `json:"meals"`
	Favorites  []Favorite        `json:"favorites"`
	Exercise   []ExerciseSession `json:"exercise"`
	Canonical  []CanonicalFood   `json:"canonical_foods"`
}
