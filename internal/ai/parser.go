package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jimgcampbell/food/docs"
	"github.com/jimgcampbell/food/internal/food"
	"github.com/jimgcampbell/food/internal/nutrition"
)

// maxToolRounds caps the agentic tool-use loop before record_meal is forced.
const maxToolRounds = 8

// USDASearcher is the subset of *nutrition.FDCClient the parser calls.
type USDASearcher interface {
	Search(ctx context.Context, query string, pageSize int) ([]nutrition.FDCFood, error)
}

// BarcodeLookuper is the subset of *nutrition.OFFClient the parser calls.
type BarcodeLookuper interface {
	Lookup(ctx context.Context, barcode string) (*nutrition.Product, error)
}

// Parser runs one agentic Claude conversation per meal parse: send messages,
// execute any tool calls, repeat until record_meal is called (or the round
// cap forces it).
type Parser struct {
	client Messenger
	usda   USDASearcher
	off    BarcodeLookuper
	log    *slog.Logger
}

func NewParser(client Messenger, usda USDASearcher, off BarcodeLookuper, log *slog.Logger) *Parser {
	return &Parser{client: client, usda: usda, off: off, log: log}
}

// ParseText parses a casual text description of a meal.
func (p *Parser) ParseText(ctx context.Context, text, day string) (*food.ParseResult, error) {
	return p.Parse(ctx, UserMessage(TextBlock(text)), day)
}

// Parse runs the tool loop starting from a prepared first user message --
// text today, a vision message (image + hint) in phase 4.
func (p *Parser) Parse(ctx context.Context, first Message, day string) (*food.ParseResult, error) {
	system := p.systemPrompt(day)
	messages := []Message{first}

	for round := 0; round < maxToolRounds; round++ {
		resp, err := p.client.CreateMessage(ctx, Request{System: system, Messages: messages, Tools: tools()})
		if err != nil {
			return nil, fmt.Errorf("ai parse round %d: %w", round+1, err)
		}
		messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content})

		if in, ok := extractRecordMeal(resp.Content); ok {
			return p.finish(in, resp.Model, messages)
		}

		toolResults, calledAny := p.executeTools(ctx, resp.Content)
		if !calledAny {
			messages = append(messages, UserMessage(TextBlock(
				"Continue. When you have enough information, call record_meal to finish.")))
			continue
		}
		messages = append(messages, UserMessage(toolResults...))
	}

	messages = append(messages, UserMessage(TextBlock(
		"You've used all available lookup rounds. Call record_meal now with your best assessment given everything so far -- estimate anything still uncertain.")))
	resp, err := p.client.CreateMessage(ctx, Request{
		System:     system,
		Messages:   messages,
		Tools:      []Tool{{Name: toolRecordMeal, Description: "Submit the final parsed meal.", InputSchema: recordMealSchema}},
		ToolChoice: &ToolChoice{Type: "tool", Name: toolRecordMeal},
	})
	if err != nil {
		return nil, fmt.Errorf("ai parse forced record_meal: %w", err)
	}
	messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content})

	in, ok := extractRecordMeal(resp.Content)
	if !ok {
		return nil, fmt.Errorf("ai did not return a meal record")
	}
	return p.finish(in, resp.Model, messages)
}

type recordMealInput struct {
	Items []food.MealItem `json:"items"`
	Notes string          `json:"notes"`
}

func extractRecordMeal(content []json.RawMessage) (*recordMealInput, bool) {
	for _, b := range decodeBlocks(content) {
		if b.Type != "tool_use" || b.Name != toolRecordMeal {
			continue
		}
		var in recordMealInput
		if err := json.Unmarshal(b.Input, &in); err != nil {
			return nil, false
		}
		return &in, true
	}
	return nil, false
}

// executeTools runs every tool_use block in content and returns the matching
// tool_result blocks, in order. calledAny is false when content had no tool
// calls at all (the model just talked).
func (p *Parser) executeTools(ctx context.Context, content []json.RawMessage) (results []json.RawMessage, calledAny bool) {
	for _, b := range decodeBlocks(content) {
		if b.Type != "tool_use" {
			continue
		}
		calledAny = true
		results = append(results, p.executeTool(ctx, b))
	}
	return results, calledAny
}

func (p *Parser) executeTool(ctx context.Context, block blockMeta) json.RawMessage {
	switch block.Name {
	case toolUSDASearch:
		return p.execUSDASearch(ctx, block)
	case toolOFFBarcode:
		return p.execOFFBarcode(ctx, block)
	default:
		return ToolResultBlock(block.ID, fmt.Sprintf("unknown tool %q", block.Name), true)
	}
}

func (p *Parser) execUSDASearch(ctx context.Context, block blockMeta) json.RawMessage {
	var in struct {
		Query    string `json:"query"`
		PageSize int    `json:"page_size"`
	}
	if err := json.Unmarshal(block.Input, &in); err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("invalid usda_search input: %v", err), true)
	}

	results, err := p.usda.Search(ctx, in.Query, in.PageSize)
	if err != nil {
		p.log.Warn("usda_search failed", "query", in.Query, "error", err)
		return ToolResultBlock(block.ID, fmt.Sprintf("usda_search failed: %v -- estimate this item instead.", err), true)
	}
	if len(results) == 0 {
		return ToolResultBlock(block.ID, "no USDA results found -- estimate this item instead.", false)
	}
	body, err := json.Marshal(results)
	if err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("failed to encode usda_search results: %v", err), true)
	}
	return ToolResultBlock(block.ID, string(body), false)
}

func (p *Parser) execOFFBarcode(ctx context.Context, block blockMeta) json.RawMessage {
	var in struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(block.Input, &in); err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("invalid off_barcode input: %v", err), true)
	}

	product, err := p.off.Lookup(ctx, in.Code)
	if err != nil {
		p.log.Warn("off_barcode lookup failed", "code", in.Code, "error", err)
		return ToolResultBlock(block.ID, fmt.Sprintf("off_barcode failed: %v -- estimate this item instead.", err), true)
	}
	body, err := json.Marshal(product)
	if err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("failed to encode off_barcode result: %v", err), true)
	}
	return ToolResultBlock(block.ID, string(body), false)
}

// finish validates every item (Atwater warnings and enum/range errors both
// fold into notes and clamp confidence to low -- the AI never writes to the
// DB, so a rough item just becomes something Jim edits or deletes in the
// draft preview) and packages the trace as ai_raw.
func (p *Parser) finish(in *recordMealInput, model string, messages []Message) (*food.ParseResult, error) {
	var extraNotes []string
	for i := range in.Items {
		it := &in.Items[i]
		it.Position = i
		if it.FractionPct == 0 {
			it.FractionPct = 100
		}
		if it.Source == "" {
			it.Source = food.SourceAI
		}
		if it.Confidence == "" {
			it.Confidence = food.ConfidenceMedium
		}
		errs, warnings := food.ValidateItem(*it)
		if len(errs) > 0 {
			it.Confidence = food.ConfidenceLow
			extraNotes = append(extraNotes, fmt.Sprintf("%s: %s", it.Name, strings.Join(errs, "; ")))
		}
		if len(warnings) > 0 {
			it.Confidence = food.ConfidenceLow
			extraNotes = append(extraNotes, warnings...)
		}
	}

	notes := strings.TrimSpace(in.Notes)
	if len(extraNotes) > 0 {
		notes = strings.TrimSpace(notes + " " + strings.Join(extraNotes, " "))
	}

	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal ai trace: %w", err)
	}

	return &food.ParseResult{
		Items:   in.Items,
		Notes:   notes,
		AIModel: model,
		AIRaw:   raw,
	}, nil
}

func (p *Parser) systemPrompt(day string) string {
	return fmt.Sprintf(`You are the meal-parsing assistant for Jim's personal food and weight tracker. You turn a casual description (typed, dictated, or from a photo) of what he ate into structured, nutrition-grounded meal items. This is a one-shot parse, not a conversation: never ask a clarifying question, just make the most reasonable assumption and say so in notes.

Jim is logging this meal for day: %s.

DIET FRAMEWORK -- the source of truth for the "tier" field on every item. Apply it verbatim; do not reinterpret or add your own nutrition opinions on top of it.

%s

TIERS
- hard_yes = 100, soft_yes = 75, neutral = 50, soft_no = 25, hard_no = 0.
- "neutral" means the framework above does not address this food at all. Do not stretch a food into hard_yes/hard_no just because it generally feels healthy or unhealthy -- use neutral when it's genuinely unaddressed, and explain your tier choice briefly in tier_reason either way.

UNITS -- integers only, never floats, in every numeric field you return
- calories: integer kcal, for the FULL portion (not as-eaten)
- protein_mg, carbs_mg, fat_mg, fiber_mg, sat_fat_mg, sugar_mg, sodium_mg: integer milligrams, for the FULL portion
- grams: integer grams, estimated full-portion weight (omit/null if you can't estimate it)
- fraction_pct: integer percent of the full portion actually eaten, 1-100, default 100

TOOLS AND ESTIMATION
- Prefer usda_search for whole foods and common dishes (e.g. "grilled chicken breast", "banana", "brown rice"). Prefer Foundation/SR Legacy results over Branded when both are plausible matches.
- Use off_barcode when barcode digits are visible in a photo or given directly.
- Estimate from your own knowledge only when a lookup fails, returns nothing useful, or the food is a composite homemade dish that no database entry represents well (e.g. "chicken stir fry with vegetables"). In that case set source to "ai" and use confidence "medium" or "low" as appropriate.
- For a photographed nutrition label, read the numbers directly off the label and set source to "label".

PORTIONS
- A hint like "I had half of this" or "just the salmon" sets fraction_pct on the affected item(s) (e.g. 50) -- it does NOT mean you should pre-scale calories/macros. Always report FULL-PORTION nutrition values and let fraction_pct carry the eaten fraction.

FINISHING
- Always state every assumption you made (portion size, preparation method, ingredient substitutions, ambiguous wording) in notes, even if brief.
- Call record_meal exactly once, as your final action, to submit the parsed items.`, day, docs.DietFramework)
}
