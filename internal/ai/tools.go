package ai

import "encoding/json"

const (
	toolUSDASearch      = "usda_search"
	toolOFFBarcode      = "off_barcode"
	toolCanonicalLookup = "canonical_lookup"
	toolRecordMeal      = "record_meal"
	toolLogExercise     = "log_exercise"
)

var canonicalLookupSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "description": "The food name to look up, e.g. 'Orgain protein powder' or 'deluxe mixed nuts'."
    }
  },
  "required": ["name"]
}`)

// canonicalLookupTool is offered only when a canonical store is wired.
func canonicalLookupTool() Tool {
	return Tool{
		Name:        toolCanonicalLookup,
		Description: "Check whether a food was already resolved and cached from a previous log. Call this FIRST for each item. On a hit you get its FDC id, data_type, per-100g nutrition, quality tier, and any existing inclusion-component tags — record the food directly (source_ref + fdc_data_type + grams) without usda_search, and copy component_tags into your components field VERBATIM (same component_id + grams_per_serving) rather than re-deciding. On a miss, resolve normally.",
		InputSchema: canonicalLookupSchema,
	}
}

// componentRulesPrompt is the encoded inclusion-component tagging guidance
// (inclusion-spec-20260726.md §5, phase-2 build prompt §1), shared verbatim
// between the meal parser's system prompt and the batch tag-suggestion call
// so the two never drift into different rules for the same five components.
const componentRulesPrompt = `INCLUSION COMPONENTS — tag foods that contribute to Jim's five tracked dietary-inclusion components, on top of (never instead of) the tier you already assign. A wrong tag silently inflates a score Jim can't see the inputs of; a missing one is visible and harmless. When in doubt, omit.

- leafy_greens and cruciferous are separate components with separate rationale. Kale (and other foods that are genuinely both, e.g. collard greens) gets BOTH tags — double-counting here is deliberate, not a bug.
- Whole fruit (peach, apple, banana, melon, etc.) gets NO component. berries is only actual berries (strawberries, blueberries, raspberries, blackberries) — never generalize it to "fruit."
- Starchy vegetables (potato, sweet potato, winter squash, corn) get NO component — the framework treats them separately from leafy/cruciferous vegetables.
- Nuts, seeds, nut butters, olive oil, and avocado get NO component. There is no such component and there will not be one — never invent one.
- fatty_fish is ONLY salmon, sardines, mackerel, herring, and anchovies. Lean fish (cod, halibut, tilapia, sole, tuna) is NOT fatty_fish and gets no component from this list.
- For a composite/prepared dish, tag only the component-bearing ingredient's share, expressed as grams_per_serving on the WHOLE dish (e.g. a lentil soup where ~400g of the dish carries one legume serving is legumes @ grams_per_serving 400 -- not the raw lentil weight).
- Default grams_per_serving when nothing better is known: leafy_greens 30 (raw) or 85 (cooked), berries 75, legumes 90 (cooked), fatty_fish 100, cruciferous 85. Prefer a household-measure portion from search/lookup results when one is available.
- Omit components entirely for a food that plausibly contributes to none — most foods do. Never tag speculatively.`

var usdaSearchSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "description": "Food name or description to search for, e.g. 'raw salmon' or 'whole wheat bread'."
    },
    "page_size": {
      "type": "integer",
      "description": "Number of results to return (default 3, max 10)."
    }
  },
  "required": ["query"]
}`)

var offBarcodeSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "code": {
      "type": "string",
      "description": "The barcode/UPC digits, e.g. '0016000275270'."
    }
  },
  "required": ["code"]
}`)

var recordMealSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "items": {
      "type": "array",
      "description": "Every food item in the meal.",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "brand": { "type": ["string", "null"] },
          "quantity": { "type": "string", "description": "Human description, e.g. '1 cup' or '2 slices'." },
          "grams": { "type": ["integer", "null"], "description": "Full-portion weight in grams. For usda/off items this DRIVES the nutrition: the app multiplies the FDC/label per-100g values by these grams, so resolve it from the search result's 'portions' household-measure weights when available (e.g. '1 cup' -> 140 g) rather than guessing. Required for usda/off items." },
          "fraction_pct": {
            "type": "integer",
            "description": "Percent of the full portion actually eaten, 1-300, default 100. A hint like 'half of this' sets this to 50; 'I had three of these' sets it to 300 -- never pre-scale the nutrition values below."
          },
          "calories": { "type": "integer", "description": "Full-portion kcal, integer. OMIT for usda/off items -- the app computes it from the FDC/label per-100g times grams. Provide it only for ai/label/web items." },
          "protein_mg": { "type": "integer", "description": "Full-portion protein in milligrams. OMIT for usda/off items (app computes). Provide for ai/label/web." },
          "carbs_mg": { "type": "integer", "description": "Full-portion carbohydrates in milligrams. OMIT for usda/off (app computes)." },
          "fat_mg": { "type": "integer", "description": "Full-portion total fat in milligrams. OMIT for usda/off (app computes)." },
          "fiber_mg": { "type": "integer", "description": "Full-portion fiber in milligrams. OMIT for usda/off (app computes)." },
          "sat_fat_mg": { "type": "integer", "description": "Full-portion saturated fat in milligrams. OMIT for usda/off (app computes)." },
          "sugar_mg": { "type": "integer", "description": "Full-portion sugar in milligrams. OMIT for usda/off (app computes)." },
          "sodium_mg": { "type": "integer", "description": "Full-portion sodium in milligrams. OMIT for usda/off (app computes)." },
          "tier": {
            "type": "string",
            "enum": ["hard_yes", "soft_yes", "neutral", "soft_no", "hard_no"],
            "description": "Quality tier per the diet framework. Use 'neutral' only when the framework's Default Rule cascade genuinely leaves a food unaddressed, not on a lookup miss."
          },
          "tier_reason": { "type": "string", "description": "Five words or fewer, citing the diet framework." },
          "tier_source": {
            "type": "string",
            "enum": ["table", "cascade"],
            "description": "'table' if the food is explicitly listed in the framework; 'cascade' if you applied the Default Rule decision cascade for an unlisted food."
          },
          "source": {
            "type": "string",
            "enum": ["usda", "off", "ai", "label", "manual", "web"],
            "description": "Where the nutrition numbers came from: usda_search, off_barcode, a photographed label, published nutrition found via web search (web), or your own estimate (ai)."
          },
          "source_ref": { "type": ["string", "null"], "description": "For usda: the FDC id (digits only). For off: the barcode. For web: the source URL. The app parses the FDC id from here to compute nutrition, so it must be exactly the chosen result's fdc_id." },
          "fdc_data_type": { "type": ["string", "null"], "description": "For usda items, the chosen search result's data_type, verbatim: 'Foundation', 'SR Legacy', 'Survey (FNDDS)', or 'Branded'. Null for non-usda." },
          "alternative_fdc_ids": { "type": "array", "items": { "type": "string" }, "description": "For usda items: the fdc_ids (digits only) of the OTHER plausible search results you considered but didn't pick, best first, up to 3. The app shows these as one-tap alternatives so Jim can fix a wrong match. Omit for non-usda items." },
          "confidence": { "type": "string", "enum": ["high", "medium", "low"] },
          "components": {
            "type": "array",
            "description": "Diet-framework inclusion components this food contributes to, or omitted/empty when it contributes to none. Most foods contribute to none.",
            "items": {
              "type": "object",
              "properties": {
                "component_id": { "type": "string", "enum": ["leafy_greens", "berries", "legumes", "fatty_fish", "cruciferous"] },
                "grams_per_serving": { "type": "integer", "description": "Grams of THIS food that make one serving of that component. For a whole food use the standard serving weight; for a composite dish use the grams of the dish that carry one serving (e.g. 400 g of lentil soup = 1 legume serving)." }
              },
              "required": ["component_id", "grams_per_serving"]
            }
          }
        },
        "required": [
          "name", "quantity", "fraction_pct", "tier", "tier_reason", "tier_source", "source", "confidence"
        ]
      }
    },
    "notes": {
      "type": "string",
      "description": "At most one short sentence, and only for a non-obvious assumption (unusual portion guess, ambiguous wording, failed lookup). Empty string when the parse was straightforward. Never re-list the items."
    }
  },
  "required": ["items", "notes"]
}`)

var logExerciseSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "sessions": {
      "type": "array",
      "description": "Every workout/practice session described in the input -- most inputs are one session, but e.g. 'hike then a swim' or 'yoga twice today' yield more than one.",
      "items": {
        "type": "object",
        "properties": {
          "type": {
            "type": "string",
            "enum": ["cardio", "strength", "yoga", "meditation", "pt"],
            "description": "cardio: running/biking/hiking/swimming/rowing. strength: lifting/gym. yoga: any yoga practice. meditation: meditation/breathwork. pt: physical therapy/rehab exercises/stretches."
          },
          "activity": { "type": ["string", "null"], "description": "Cardio only. Map casual phrasing to the known vocab when obvious: Run, Ride (outdoor cycling), Spin (stationary/indoor bike, spin class, Peloton), Hike, Swim, Row, Other." },
          "location": { "type": ["string", "null"], "description": "Yoga only: where it happened, e.g. 'Studio' or 'Home'. Strength captures any location in note instead." },
          "style": { "type": ["string", "null"], "description": "Yoga only. Map to the known vocab when obvious: Vinyasa, Hot, Other." },
          "duration_min": { "type": ["integer", "null"], "description": "Integer minutes, from whatever the input stated. Required for cardio/yoga/meditation; must be null for strength and pt (pt is a binary did-it/didn't habit -- put any minutes or detail in note instead). Leave null if truly unstated -- don't guess." },
          "note": { "type": "string", "description": "Optional free-text note, empty string if none." }
        },
        "required": ["type"]
      }
    }
  },
  "required": ["sessions"]
}`)

// webSearchTool is Anthropic's server-side web search: executed API-side
// mid-request, so restaurant/chain nutrition can come from the publisher's
// own pages. MaxUses caps searches per parse.
func webSearchTool() Tool {
	return Tool{Type: "web_search_20250305", Name: "web_search", MaxUses: 3}
}

func tools() []Tool {
	return []Tool{
		{
			Name:        toolUSDASearch,
			Description: "Search USDA FoodData Central for a whole food, composite/prepared dish, or packaged product (Foundation, SR Legacy, Survey/FNDDS, and Branded tiers). Prefer this over estimating for anything but a homemade composite no database represents. Results include per-100g nutrients and a 'portions' list of household-measure gram weights -- use those to resolve grams. Call it once per item, batching all of a meal's searches into a single response.",
			InputSchema: usdaSearchSchema,
		},
		{
			Name:        toolOFFBarcode,
			Description: "Look up a packaged product by its barcode/UPC in Open Food Facts. Use when digits are visible or known.",
			InputSchema: offBarcodeSchema,
		},
		{
			Name:        toolRecordMeal,
			Description: "Terminal tool for FOOD: submit the final parsed meal. Call this OR log_exercise, never both -- exactly one terminal tool call to finish, even if some items are estimates. Your response must contain only this tool call -- no text before or after it.",
			InputSchema: recordMealSchema,
		},
		{
			Name:        toolLogExercise,
			Description: "Terminal tool for EXERCISE/workouts: submit the parsed session(s). Call this OR record_meal, never both -- exactly one terminal tool call to finish. No USDA/barcode lookups needed first. Your response must contain only this tool call -- no text before or after it.",
			InputSchema: logExerciseSchema,
		},
	}
}
