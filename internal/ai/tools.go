package ai

import "encoding/json"

const (
	toolUSDASearch  = "usda_search"
	toolOFFBarcode  = "off_barcode"
	toolRecordMeal  = "record_meal"
	toolLogExercise = "log_exercise"
)

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
          "grams": { "type": ["integer", "null"], "description": "Estimated full-portion weight in grams." },
          "fraction_pct": {
            "type": "integer",
            "description": "Percent of the full portion actually eaten, 1-100, default 100. A hint like 'half of this' sets this to 50 -- never pre-scale the nutrition values below."
          },
          "calories": { "type": "integer", "description": "Full-portion kcal, integer." },
          "protein_mg": { "type": "integer", "description": "Full-portion protein in milligrams." },
          "carbs_mg": { "type": "integer", "description": "Full-portion carbohydrates in milligrams." },
          "fat_mg": { "type": "integer", "description": "Full-portion total fat in milligrams." },
          "fiber_mg": { "type": "integer", "description": "Full-portion fiber in milligrams." },
          "sat_fat_mg": { "type": "integer", "description": "Full-portion saturated fat in milligrams." },
          "sugar_mg": { "type": "integer", "description": "Full-portion sugar in milligrams." },
          "sodium_mg": { "type": "integer", "description": "Full-portion sodium in milligrams." },
          "tier": {
            "type": "string",
            "enum": ["hard_yes", "soft_yes", "neutral", "soft_no", "hard_no"],
            "description": "Quality tier per the diet framework. Use 'neutral' only when the framework doesn't address this food."
          },
          "tier_reason": { "type": "string", "description": "Five words or fewer, citing the diet framework." },
          "source": {
            "type": "string",
            "enum": ["usda", "off", "ai", "label", "manual", "web"],
            "description": "Where the nutrition numbers came from: usda_search, off_barcode, a photographed label, published nutrition found via web search (web), or your own estimate (ai)."
          },
          "source_ref": { "type": ["string", "null"], "description": "FDC id, barcode, or source URL for web results." },
          "confidence": { "type": "string", "enum": ["high", "medium", "low"] }
        },
        "required": [
          "name", "quantity", "fraction_pct", "calories", "protein_mg", "carbs_mg", "fat_mg",
          "fiber_mg", "sat_fat_mg", "sugar_mg", "sodium_mg", "tier", "tier_reason", "source", "confidence"
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
          "activity": { "type": ["string", "null"], "description": "Cardio only. Map casual phrasing to the known vocab when obvious: Run, Bike, Hike, Swim, Row, Other." },
          "location": { "type": ["string", "null"], "description": "Strength/yoga only: where it happened, e.g. a gym name, 'Studio', 'Home'." },
          "style": { "type": ["string", "null"], "description": "Yoga only. Map to the known vocab when obvious: Vinyasa, Hot, Other." },
          "duration_min": { "type": ["integer", "null"], "description": "Integer minutes, from whatever the input stated. Required for cardio/yoga/meditation/pt; must be null for strength (no duration field for it yet). Leave null if truly unstated -- don't guess." },
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
			Description: "Search USDA FoodData Central for a whole food or common dish. Prefer this over estimating for anything not a homemade composite dish. Call it once per item, batching all of a meal's searches into a single response.",
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
