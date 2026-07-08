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
)

const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
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

var validTiers = map[string]bool{
	TierHardYes: true, TierSoftYes: true, TierNeutral: true, TierSoftNo: true, TierHardNo: true,
}
var validSources = map[string]bool{
	SourceUSDA: true, SourceOFF: true, SourceAI: true, SourceLabel: true, SourceManual: true,
}
var validConfidences = map[string]bool{
	ConfidenceHigh: true, ConfidenceMedium: true, ConfidenceLow: true,
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
}

// Weight is one day's weigh-in; a second entry for the same day replaces it.
type Weight struct {
	Day     string `json:"day"` // YYYY-MM-DD
	WeightG int    `json:"weight_g"`
	Note    string `json:"note"`
}

// Settings is the single-row (id=1) app configuration.
type Settings struct {
	CalorieTarget   int       `json:"calorie_target"`
	ProteinTargetMg int64     `json:"protein_target_mg"`
	WeightTargetG   *int      `json:"weight_target_g,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// DaySummary is the as-eaten totals and quality score for one day, plus the
// day's meals. Score is nil when the day has no calorie-bearing items.
type DaySummary struct {
	Day                string `json:"day"`
	Calories           int64  `json:"calories"`
	ProteinMg          int64  `json:"protein_mg"`
	CarbsMg            int64  `json:"carbs_mg"`
	FatMg              int64  `json:"fat_mg"`
	FiberMg            int64  `json:"fiber_mg"`
	Score              *int   `json:"score"`
	CalorieTarget      int    `json:"calorie_target"`
	ProteinTargetMg    int64  `json:"protein_target_mg"`
	CaloriesRemaining  int64  `json:"calories_remaining"`
	ProteinRemainingMg int64  `json:"protein_remaining_mg"`
	Meals              []Meal `json:"meals"`
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

// ParseResult is the draft returned by the AI parse endpoints (phase 3+).
// Items use the same shape as MealItem so the client can edit the draft and
// POST it to /api/meals unchanged.
type ParseResult struct {
	Items   []MealItem      `json:"items"`
	Notes   string          `json:"notes"`
	AIModel string          `json:"ai_model"`
	AIRaw   json.RawMessage `json:"ai_raw,omitempty"`
}

// ExportDoc is the full-database backup returned by GET /api/export — the
// backup story for Render's ephemeral disk. Meals are ordered by day.
type ExportDoc struct {
	ExportedAt time.Time `json:"exported_at"`
	Settings   Settings  `json:"settings"`
	Weights    []Weight  `json:"weights"`
	Meals      []Meal    `json:"meals"`
}
