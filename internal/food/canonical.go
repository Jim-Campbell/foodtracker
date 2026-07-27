package food

import (
	"context"
	"strings"
	"time"
)

// Per100 is a food's nutrition per 100 grams, integer units — the reusable
// basis stored in the canonical table. As-logged nutrition for any portion is
// these values * grams / 100.
type Per100 struct {
	Calories  int64 `json:"calories"`
	ProteinMg int64 `json:"protein_mg"`
	CarbsMg   int64 `json:"carbs_mg"`
	FatMg     int64 `json:"fat_mg"`
	FiberMg   int64 `json:"fiber_mg"`
	SatFatMg  int64 `json:"sat_fat_mg"`
	SugarMg   int64 `json:"sugar_mg"`
	SodiumMg  int64 `json:"sodium_mg"`
}

// CanonicalFood is one accreted food: a normalized name mapped to its resolved
// per-100g nutrition, provenance, and quality tier. Built by accretion as meals
// are saved and reused on later logs of the same food so it never re-resolves.
type CanonicalFood struct {
	ID              int64     `json:"id"`
	NormalizedName  string    `json:"normalized_name"`
	DisplayName     string    `json:"display_name"`
	Source          string    `json:"source"`
	SourceRef       *string   `json:"source_ref,omitempty"`
	FDCID           *int64    `json:"fdc_id,omitempty"`
	FDCDataType     *string   `json:"fdc_data_type,omitempty"`
	ResolutionTier  *int      `json:"resolution_tier,omitempty"`
	Per100g         Per100    `json:"per_100g"`
	DefaultGrams    *int      `json:"default_grams,omitempty"`
	Tier            string    `json:"tier"`
	TierReason      string    `json:"tier_reason"`
	TierSource      *string   `json:"tier_source,omitempty"`
	Protected       bool      `json:"protected"`
	ProtectedReason string    `json:"protected_reason,omitempty"`
	TimesLogged     int       `json:"times_logged"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"`
}

// NormalizeFoodName is the canonical-table key: lower-cased and
// whitespace-collapsed, so "Deluxe Mixed Nuts" and "deluxe  mixed nuts" map to
// one entry. Intentionally conservative — it collapses casing and spacing noise
// without trying to equate genuinely different names ("mixed nuts" stays
// distinct from "deluxe mixed nuts").
func NormalizeFoodName(name string) string {
	return strings.Join(strings.Fields(strings.ToLower(name)), " ")
}

// canonicalFromItem derives a canonical entry from a just-saved meal item, or
// returns ok=false when the item isn't worth accreting. Only grounded items
// (usda/off/label/web) with a known portion weight qualify, since per-100g is
// back-computed from the item's full-portion nutrition and grams. Pure AI
// estimates and portionless items are skipped — they're not a reusable basis.
func canonicalFromItem(it MealItem) (CanonicalFood, bool) {
	name := strings.TrimSpace(it.Name)
	if name == "" {
		return CanonicalFood{}, false
	}
	if it.Source == SourceAI || it.Source == SourceManual {
		return CanonicalFood{}, false
	}
	if it.Grams == nil || *it.Grams <= 0 {
		return CanonicalFood{}, false
	}
	g := int64(*it.Grams)
	per := Per100{
		Calories:  it.Calories * 100 / g,
		ProteinMg: it.ProteinMg * 100 / g,
		CarbsMg:   it.CarbsMg * 100 / g,
		FatMg:     it.FatMg * 100 / g,
		FiberMg:   it.FiberMg * 100 / g,
		SatFatMg:  it.SatFatMg * 100 / g,
		SugarMg:   it.SugarMg * 100 / g,
		SodiumMg:  it.SodiumMg * 100 / g,
	}
	grams := *it.Grams
	return CanonicalFood{
		NormalizedName: NormalizeFoodName(name),
		DisplayName:    name,
		Source:         it.Source,
		SourceRef:      it.SourceRef,
		FDCID:          it.FDCID,
		FDCDataType:    it.FDCDataType,
		ResolutionTier: it.ResolutionTier,
		Per100g:        per,
		DefaultGrams:   &grams,
		Tier:           it.Tier,
		TierReason:     it.TierReason,
		TierSource:     it.TierSource,
	}, true
}

// accreteCanonical writes each eligible saved item into the canonical table so
// the same food reuses its resolved numbers next time. Failures are logged and
// swallowed — accretion is a cache-warming side effect and must never fail a
// meal save.
func (s *Service) accreteCanonical(ctx context.Context, items []MealItem) {
	for _, it := range items {
		cf, ok := canonicalFromItem(it)
		if !ok {
			continue
		}
		if err := s.store.UpsertCanonical(ctx, &cf); err != nil {
			s.log.Warn("canonical accretion failed", "food", cf.DisplayName, "error", err)
		}
	}
}

// accreteComponentTags fans each saved item's proposed/confirmed inclusion
// component tags out into food_component_tags — "tag once, persist, reuse
// forever" (inclusion-spec-20260726.md §5). Unlike accreteCanonical this runs
// for every source, including ai/manual composite dishes: a homemade lentil
// soup is exactly the kind of food this table exists to capture, and the
// model never writes to the DB itself, so a save is the only place a proposal
// becomes a durable tag. Failures are logged and swallowed, same as canonical
// accretion.
func (s *Service) accreteComponentTags(ctx context.Context, items []MealItem) {
	for _, it := range items {
		if len(it.Components) == 0 {
			continue
		}
		name := NormalizeFoodName(strings.TrimSpace(it.Name))
		if name == "" {
			continue
		}
		for _, c := range it.Components {
			source := c.Source
			if source == "" {
				source = TagSourceAI
			}
			tag := ComponentTag{NormalizedName: name, FDCID: it.FDCID, ComponentID: c.ComponentID, GramsPerServing: c.GramsPerServing}
			if err := s.store.UpsertComponentTag(ctx, tag, source); err != nil {
				s.log.Warn("component tag accretion failed", "food", it.Name, "component", c.ComponentID, "error", err)
			}
		}
	}
}
