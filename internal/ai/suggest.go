package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jimgcampbell/food/internal/food"
)

const toolSuggestTags = "suggest_tags"

var suggestTagsSchema = json.RawMessage(`{
  "type": "object",
  "properties": {
    "foods": {
      "type": "array",
      "description": "One entry per food name given, in the same order.",
      "items": {
        "type": "object",
        "properties": {
          "name": { "type": "string" },
          "components": {
            "type": "array",
            "description": "Diet-framework inclusion components this food contributes to, or empty when it contributes to none. Most foods contribute to none.",
            "items": {
              "type": "object",
              "properties": {
                "component_id": { "type": "string", "enum": ["leafy_greens", "berries", "legumes", "fatty_fish", "cruciferous"] },
                "grams_per_serving": { "type": "integer" }
              },
              "required": ["component_id", "grams_per_serving"]
            }
          }
        },
        "required": ["name", "components"]
      }
    }
  },
  "required": ["foods"]
}`)

// SuggestedFoodTags is one food's proposed component tags from a batch
// suggestion call (inclusion phase 2 §4, Settings → "Tag foods"). Nothing is
// persisted here -- the PWA renders these as unconfirmed chips Jim accepts or
// rejects, same AI-never-writes invariant as every other draft.
type SuggestedFoodTags struct {
	Name       string               `json:"name"`
	Components []food.ItemComponent `json:"components"`
}

// SuggestComponentTags runs ONE Claude call over a batch of untagged food
// names and proposes inclusion-component tags for each, so the backfill
// screen doesn't need one call per food. Uses the same component rules as the
// meal parser (componentRulesPrompt) so the two never diverge.
func (p *Parser) SuggestComponentTags(ctx context.Context, names []string) ([]SuggestedFoodTags, error) {
	if len(names) == 0 {
		return nil, nil
	}
	var list strings.Builder
	for _, n := range names {
		list.WriteString("- ")
		list.WriteString(n)
		list.WriteString("\n")
	}
	prompt := "Propose inclusion-component tags for every one of these foods, in order, one entry per name (empty components array when a food plausibly has none):\n" + list.String()

	resp, err := p.client.CreateMessage(ctx, Request{
		System:     CachedSystem("You tag foods with dietary-inclusion components for Jim's food tracker.\n\n" + componentRulesPrompt),
		Messages:   []Message{UserMessage(TextBlock(prompt))},
		Tools:      []Tool{{Name: toolSuggestTags, Description: "Submit your tag proposals for every food listed.", InputSchema: suggestTagsSchema}},
		ToolChoice: &ToolChoice{Type: "tool", Name: toolSuggestTags},
	})
	if err != nil {
		return nil, fmt.Errorf("suggest component tags: %w", err)
	}

	for _, b := range decodeBlocks(resp.Content) {
		if b.Type != "tool_use" || b.Name != toolSuggestTags {
			continue
		}
		var in struct {
			Foods []SuggestedFoodTags `json:"foods"`
		}
		if err := json.Unmarshal(b.Input, &in); err != nil {
			return nil, fmt.Errorf("decode suggest_tags input: %w", err)
		}
		for i := range in.Foods {
			in.Foods[i].Components = food.SanitizeItemComponents(in.Foods[i].Components)
		}
		return in.Foods, nil
	}
	return nil, fmt.Errorf("ai did not return tag suggestions")
}
