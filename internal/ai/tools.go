package ai

import "encoding/json"

const (
	toolUSDASearch = "usda_search"
	toolOFFBarcode = "off_barcode"
	toolRecordMeal = "record_meal"
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
            "enum": ["usda", "off", "ai", "label", "manual"],
            "description": "Where the nutrition numbers came from: usda_search, off_barcode, a photographed label, or your own estimate (ai)."
          },
          "source_ref": { "type": ["string", "null"], "description": "FDC id or barcode, if applicable." },
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
			Description: "Terminal tool: submit the final parsed meal. Always call this exactly once to finish, even if some items are estimates.",
			InputSchema: recordMealSchema,
		},
	}
}
