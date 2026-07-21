package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/jimgcampbell/food/docs"
	"github.com/jimgcampbell/food/internal/food"
	"github.com/jimgcampbell/food/internal/nutrition"
)

// microsCache accumulates full nutrient payloads from tool executions during
// one parse, keyed "<source>:<source_ref>" (e.g. "usda:173735", "off:001600...").
// The model only ever sees slim tool results; finish() attaches these to the
// matching items server-side, so meal_items.micros stays complete without the
// model reading or regenerating thousands of nutrient tokens.
type microsCache map[string]json.RawMessage

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
	// visionModel overrides the client's default model for photo parses.
	// Vision work (reading barcode digits, nutrition panels) is where model
	// capability shows -- a fast model that misreads a barcode logs the wrong
	// food entirely. Empty means use the client default.
	visionModel string
	// webSearch exposes Anthropic's server-side web search to the parse loop
	// so chain-restaurant items get the chain's published nutrition.
	webSearch bool
}

func NewParser(client Messenger, usda USDASearcher, off BarcodeLookuper, visionModel string, webSearch bool, log *slog.Logger) *Parser {
	return &Parser{client: client, usda: usda, off: off, visionModel: visionModel, webSearch: webSearch, log: log}
}

// tools returns the parse loop's tool set, including web search when enabled.
func (p *Parser) tools() []Tool {
	ts := tools()
	if p.webSearch {
		ts = append(ts, webSearchTool())
	}
	return ts
}

// Progress receives short user-facing status lines ("Looking up “feta
// cheese”…") as the parse advances; the API layer streams them to the PWA so
// the wait narrates itself. The exported methods take the unnamed func(string)
// form (nil allowed) so *Parser satisfies the api-package interfaces.
type Progress func(msg string)

// ParseText parses a casual text description of a meal.
func (p *Parser) ParseText(ctx context.Context, text, day string, progress func(string)) (*food.ParseResult, error) {
	return p.parse(ctx, UserMessage(TextBlock(text)), day, "", progress)
}

// ParseImage parses a photo (label, barcode, package, or plate) plus an
// optional hint like "I had half of this".
func (p *Parser) ParseImage(ctx context.Context, imageData []byte, mediaType, hint, day string, progress func(string)) (*food.ParseResult, error) {
	hintText := "No hint was given -- read the photo directly and parse it (nutrition label, barcode, package, or plate of food)."
	if hint != "" {
		hintText = "Hint from Jim: " + hint
	}
	return p.parse(ctx, UserMessage(ImageBlock(mediaType, imageData), TextBlock(hintText)), day, p.visionModel, progress)
}

// parse runs the tool loop starting from a prepared first user message --
// plain text, or a vision message (image + hint). model overrides the client
// default when non-empty.
func (p *Parser) parse(ctx context.Context, first Message, day, model string, progress func(string)) (*food.ParseResult, error) {
	emit := func(msg string) {
		if progress != nil {
			progress(msg)
		}
	}
	system := CachedSystem(p.systemPrompt(day))
	messages := []Message{first}
	cache := microsCache{}
	start := time.Now()

	for round := 0; round < maxToolRounds; round++ {
		t0 := time.Now()
		resp, err := p.client.CreateMessage(ctx, Request{Model: model, System: system, Messages: messages, Tools: p.tools()})
		if err != nil {
			return nil, fmt.Errorf("ai parse round %d: %w", round+1, err)
		}
		p.logRound(round+1, t0, resp)
		messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content})

		if in, ok := extractRecordMeal(resp.Content); ok {
			p.log.Info("parse done", "rounds", round+1, "total_ms", time.Since(start).Milliseconds())
			return p.finish(in, resp.Model, messages, cache)
		}
		if in, ok := extractLogExercise(resp.Content); ok {
			p.log.Info("parse done (exercise)", "rounds", round+1, "total_ms", time.Since(start).Milliseconds())
			return p.finishExercise(in, resp.Model, messages)
		}

		// A long-running server tool (web search) pauses the turn; replay the
		// conversation as-is so the API resumes it.
		if resp.StopReason == "pause_turn" {
			emit("Searching the web…")
			continue
		}

		toolResults, calledAny := p.executeTools(ctx, resp.Content, cache, emit)
		if !calledAny {
			messages = append(messages, UserMessage(TextBlock(
				"Continue. When you have enough information, call record_meal to finish.")))
			continue
		}
		messages = append(messages, UserMessage(toolResults...))
		emit("Calculating nutrition…")
	}

	messages = append(messages, UserMessage(TextBlock(
		"You've used all available lookup rounds. Call record_meal (food) or log_exercise (workout) now with your best assessment given everything so far -- estimate anything still uncertain.")))
	t0 := time.Now()
	resp, err := p.client.CreateMessage(ctx, Request{
		Model:    model,
		System:   system,
		Messages: messages,
		Tools: []Tool{
			{Name: toolRecordMeal, Description: "Submit the final parsed meal.", InputSchema: recordMealSchema},
			{Name: toolLogExercise, Description: "Submit the final parsed workout session(s).", InputSchema: logExerciseSchema},
		},
		ToolChoice: &ToolChoice{Type: "any"},
	})
	if err != nil {
		return nil, fmt.Errorf("ai parse forced terminal tool: %w", err)
	}
	p.logRound(maxToolRounds+1, t0, resp)
	messages = append(messages, Message{Role: RoleAssistant, Content: resp.Content})

	if in, ok := extractRecordMeal(resp.Content); ok {
		p.log.Info("parse done (forced)", "rounds", maxToolRounds+1, "total_ms", time.Since(start).Milliseconds())
		return p.finish(in, resp.Model, messages, cache)
	}
	if in, ok := extractLogExercise(resp.Content); ok {
		p.log.Info("parse done (forced exercise)", "rounds", maxToolRounds+1, "total_ms", time.Since(start).Milliseconds())
		return p.finishExercise(in, resp.Model, messages)
	}
	return nil, fmt.Errorf("ai did not return a meal or exercise record")
}

// logRound records one model call's latency and token accounting; a slow
// parse can be broken down from these lines alone (model time vs tool time,
// cache hits vs cold prompt reads).
func (p *Parser) logRound(round int, started time.Time, resp *Response) {
	p.log.Info("parse round",
		"round", round,
		"model", resp.Model,
		"dur_ms", time.Since(started).Milliseconds(),
		"stop", resp.StopReason,
		"input_tokens", resp.Usage.InputTokens,
		"cache_read", resp.Usage.CacheReadInputTokens,
		"cache_write", resp.Usage.CacheCreationInputTokens,
		"output_tokens", resp.Usage.OutputTokens,
		"blocks", blockSummary(resp.Content))
}

// blockSummary renders a response's content-block composition, e.g.
// "text(412),tool_use:record_meal" -- it answers "what were those output
// tokens?" when a round looks slow.
func blockSummary(raws []json.RawMessage) string {
	var parts []string
	for _, r := range raws {
		var b struct {
			Type     string `json:"type"`
			Name     string `json:"name"`
			Text     string `json:"text"`
			Thinking string `json:"thinking"`
		}
		if err := json.Unmarshal(r, &b); err != nil {
			continue
		}
		switch b.Type {
		case "text":
			parts = append(parts, fmt.Sprintf("text(%d)", len(b.Text)))
		case "thinking":
			parts = append(parts, fmt.Sprintf("thinking(%d)", len(b.Thinking)))
		case "tool_use":
			parts = append(parts, "tool_use:"+b.Name)
		default:
			parts = append(parts, b.Type)
		}
	}
	return strings.Join(parts, ",")
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

// logExerciseInput reuses food.ExerciseSession's JSON shape directly: the
// tool schema only ever populates type/activity/location/style/duration_min/
// note, so the other fields (Day, InputKind, ...) stay zero -- the PWA fills
// those in when it saves the confirmed draft to POST /api/exercise.
type logExerciseInput struct {
	Sessions []food.ExerciseSession `json:"sessions"`
}

func extractLogExercise(content []json.RawMessage) (*logExerciseInput, bool) {
	for _, b := range decodeBlocks(content) {
		if b.Type != "tool_use" || b.Name != toolLogExercise {
			continue
		}
		var in logExerciseInput
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
func (p *Parser) executeTools(ctx context.Context, content []json.RawMessage, cache microsCache, emit Progress) (results []json.RawMessage, calledAny bool) {
	for _, b := range decodeBlocks(content) {
		if b.Type != "tool_use" {
			continue
		}
		calledAny = true
		results = append(results, p.executeTool(ctx, b, cache, emit))
	}
	return results, calledAny
}

func (p *Parser) executeTool(ctx context.Context, block blockMeta, cache microsCache, emit Progress) json.RawMessage {
	switch block.Name {
	case toolUSDASearch:
		return p.execUSDASearch(ctx, block, cache, emit)
	case toolOFFBarcode:
		return p.execOFFBarcode(ctx, block, cache, emit)
	default:
		return ToolResultBlock(block.ID, fmt.Sprintf("unknown tool %q", block.Name), true)
	}
}

func (p *Parser) execUSDASearch(ctx context.Context, block blockMeta, cache microsCache, emit Progress) json.RawMessage {
	var in struct {
		Query    string `json:"query"`
		PageSize int    `json:"page_size"`
	}
	if err := json.Unmarshal(block.Input, &in); err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("invalid usda_search input: %v", err), true)
	}
	emit(fmt.Sprintf("Looking up “%s”…", in.Query))

	t0 := time.Now()
	results, err := p.usda.Search(ctx, in.Query, in.PageSize)
	if err != nil {
		p.log.Warn("usda_search failed", "query", in.Query, "dur_ms", time.Since(t0).Milliseconds(), "error", err)
		return ToolResultBlock(block.ID, fmt.Sprintf("usda_search failed: %v -- estimate this item instead.", err), true)
	}
	p.log.Info("usda_search", "query", in.Query, "results", len(results), "dur_ms", time.Since(t0).Milliseconds())
	if len(results) == 0 {
		return ToolResultBlock(block.ID, "no USDA results found -- estimate this item instead.", false)
	}

	// Cache each result's full nutrient list for micros, then strip it from
	// what the model sees: an FDC food can carry 100+ nutrient entries, and
	// they'd be re-sent as input tokens on every remaining round.
	slim := make([]nutrition.FDCFood, len(results))
	for i, f := range results {
		if len(f.Nutrients) > 0 {
			payload, err := json.Marshal(struct {
				Nutrients []nutrition.NutrientAmount `json:"nutrients"`
			}{f.Nutrients})
			if err == nil {
				cache[food.SourceUSDA+":"+strconv.Itoa(f.FDCID)] = payload
			}
		}
		f.Nutrients = nil
		slim[i] = f
	}
	body, err := json.Marshal(slim)
	if err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("failed to encode usda_search results: %v", err), true)
	}
	return ToolResultBlock(block.ID, string(body), false)
}

func (p *Parser) execOFFBarcode(ctx context.Context, block blockMeta, cache microsCache, emit Progress) json.RawMessage {
	var in struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(block.Input, &in); err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("invalid off_barcode input: %v", err), true)
	}
	emit("Checking the barcode…")

	t0 := time.Now()
	product, err := p.off.Lookup(ctx, in.Code)
	if err != nil {
		p.log.Warn("off_barcode lookup failed", "code", in.Code, "dur_ms", time.Since(t0).Milliseconds(), "error", err)
		fallback := "identify the product from the package's printed TEXT (brand name, product name, website) -- never from the food artwork on the label"
		if p.webSearch {
			fallback = "identify the product from the package's printed TEXT (brand name, product name, website -- never the food artwork) and use web_search to find its published nutrition"
		}
		return ToolResultBlock(block.ID, fmt.Sprintf(
			"off_barcode found nothing for %q: %v -- barcode digits are easy to misread. Re-read the printed numerals under the bars digit by digit (UPC-A has 12) and try off_barcode ONCE more if you read them differently. If it still fails, %s.",
			in.Code, err, fallback), true)
	}
	p.log.Info("off_barcode", "code", in.Code, "dur_ms", time.Since(t0).Milliseconds())
	if len(product.RawNutriments) > 0 {
		cache[food.SourceOFF+":"+in.Code] = product.RawNutriments
	}
	body, err := json.Marshal(product) // RawNutriments is json:"-", so the model gets the slim view
	if err != nil {
		return ToolResultBlock(block.ID, fmt.Sprintf("failed to encode off_barcode result: %v", err), true)
	}
	return ToolResultBlock(block.ID, string(body), false)
}

// finish validates every item (Atwater warnings and enum/range errors both
// fold into notes and clamp confidence to low -- the AI never writes to the
// DB, so a rough item just becomes something Jim edits or deletes in the
// draft preview), attaches cached full nutrient payloads as micros, and
// packages the trace as ai_raw.
func (p *Parser) finish(in *recordMealInput, model string, messages []Message, cache microsCache) (*food.ParseResult, error) {
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
		if len(it.Micros) == 0 && it.SourceRef != nil && *it.SourceRef != "" {
			if payload, ok := cache[it.Source+":"+*it.SourceRef]; ok {
				it.Micros = payload
			}
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
		Kind:    food.ParseKindMeal,
		Items:   in.Items,
		Notes:   notes,
		AIModel: model,
		AIRaw:   raw,
	}, nil
}

// finishExercise validates each parsed session's per-type required fields
// (the AI never writes to the DB, so an incomplete session just becomes
// something Jim completes in the confirm sheet) and packages the trace as
// ai_raw, mirroring finish().
func (p *Parser) finishExercise(in *logExerciseInput, model string, messages []Message) (*food.ParseResult, error) {
	var extraNotes []string
	for i, s := range in.Sessions {
		if errs := food.ExerciseFieldErrors(s); len(errs) > 0 {
			extraNotes = append(extraNotes, fmt.Sprintf("session %d (%s): %s", i+1, s.Type, strings.Join(errs, "; ")))
		}
	}

	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, fmt.Errorf("marshal ai trace: %w", err)
	}

	return &food.ParseResult{
		Kind:     food.ParseKindExercise,
		Exercise: in.Sessions,
		Notes:    strings.Join(extraNotes, " "),
		AIModel:  model,
		AIRaw:    raw,
	}, nil
}

func (p *Parser) systemPrompt(day string) string {
	// The branded-product fallback depends on whether web search is available:
	// with it, escalate to the brand's published nutrition; without it, an
	// estimate is the best remaining option.
	webGuidance := `
- For branded packaged products (protein powders, bars, cereals), if the first search has no confident match, estimate from your knowledge of that product's label rather than searching USDA again.`
	if p.webSearch {
		webGuidance = `
- Restaurants and chains ("Five Guys cheeseburger", "Chipotle chicken bowl"): skip usda_search and use web_search directly for the chain's published nutrition -- chains publish exact numbers.
- Any other brand name (packaged goods, store brands, local restaurants, bakery items): if usda_search has no confident match for the specific product, use web_search to find the brand's published nutrition BEFORE falling back to an estimate -- Jim values accuracy over speed. Estimate only when the web has nothing authoritative either.
- For every web-sourced item set source to "web" and source_ref to the URL the numbers came from.`
	}
	return fmt.Sprintf(`You are the logging assistant for Jim's personal food, weight, and exercise tracker. You turn a casual description (typed, dictated, or from a photo) into structured, grounded records. This is a one-shot parse, not a conversation: never ask a clarifying question, just make the most reasonable assumption and say so in notes.

Jim is logging for day: %s.

CLASSIFY FIRST -- the app tracks both food and exercise through this one input. Decide which this input describes, then call exactly ONE terminal tool: record_meal for food/drink, log_exercise for a workout or practice ("30 min run", "hot yoga at the studio, 60 minutes", "hike after a swim", "did my PT", "lifted at the gym"). Photos are always food (a meal, label, barcode, or plate) -- never exercise. Exercise inputs need no usda_search/off_barcode lookups; fill sessions directly from the text using the log_exercise tool's field guidance (map casual phrasing to the known chip vocab where obvious -- "lifted"/"gym" -> strength, "ran"/"jog" -> cardio Run, "spin"/"cycling" -> Bike, "swam" -> Swim, "PT"/"physical therapy"/"rehab exercises"/"my stretches" -> pt; infer duration_min from stated minutes for cardio/yoga/meditation, but leave it null for strength and pt; leave a field null if truly unstated and let Jim fill it in the confirm sheet).

DIET FRAMEWORK -- the source of truth for the "tier" field on every item. Apply it verbatim; do not reinterpret or add your own nutrition opinions on top of it.

%s

TIERS
- hard_yes = 100, soft_yes = 75, neutral = 50, soft_no = 25, hard_no = 0.
- "neutral" means the framework above does not address this food at all. Do not stretch a food into hard_yes/hard_no just because it generally feels healthy or unhealthy -- use neutral when it's genuinely unaddressed. tier_reason is five words or fewer, either way.

UNITS -- integers only, never floats, in every numeric field you return
- calories: integer kcal, for the FULL portion (not as-eaten)
- protein_mg, carbs_mg, fat_mg, fiber_mg, sat_fat_mg, sugar_mg, sodium_mg: integer milligrams, for the FULL portion
- grams: integer grams, estimated full-portion weight (omit/null if you can't estimate it)
- fraction_pct: integer percent of the full portion actually eaten, 1-300, default 100

TOOLS AND ESTIMATION
- Prefer usda_search for whole foods and common dishes (e.g. "grilled chicken breast", "banana", "brown rice"). Prefer Foundation/SR Legacy results over Branded when both are plausible matches.
- Batch your lookups: issue every usda_search call (one per item) together in a single response. A typical parse is two turns total -- one batched lookup turn, then record_meal. Only take an extra turn when a first search came back empty or clearly wrong.
- Don't over-search USDA: at most one usda_search per item, plus at most one reworded retry for the whole meal.%s
- Use off_barcode when barcode digits are visible in a photo or given directly.
- Estimate from your own knowledge only when a lookup fails, returns nothing useful, or the food is a composite homemade dish that no database entry represents well (e.g. "chicken stir fry with vegetables"). In that case set source to "ai" and use confidence "medium" or "low" as appropriate.
- For a photographed nutrition label, read the numbers directly off the label and set source to "label".

PORTIONS
- A hint like "I had half of this" or "just the salmon" sets fraction_pct on the affected item(s) (e.g. 50); "I had two/three servings" sets it above 100 (e.g. 200, 300) -- it does NOT mean you should pre-scale calories/macros. Always report FULL-PORTION nutrition values and let fraction_pct carry the eaten fraction.

PHOTOS -- when the first message includes an image, decide which of these it is and follow the matching rule:
- Packaged products first rule: packaging is covered in pictures of fruit, berries, grains, and finished dishes -- that is flavor/ingredient ARTWORK, not the food. Never identify a packaged product from its artwork. Identify it from printed text only: brand name, product name, website domain, nutrition panel, barcode. A tub showing berries with "ORGAIN.COM" printed on it is Orgain protein powder, not a bag of berries.
- Barcode digits: read them from the printed numerals under the bars, digit by digit (UPC-A has 12 digits, EAN-13 has 13). Double-check before calling off_barcode.
- Nutrition label visible: transcribe the panel exactly as printed (serving size, servings per container, per-serving values). Set source to "label". Compute full-portion values from how many servings Jim actually ate: a hint like "I ate the whole box" multiplies the per-serving values by servings per container; with no hint, assume one serving and say so in notes.
- Barcode with legible digits: call off_barcode with the digits. On a hit, set source to "off" and source_ref to the barcode. On a miss, fall back to reading the package text and using usda_search.
- Package front only (no label, no barcode): identify the product from what's visible, then usda_search a Branded match or estimate; confidence is "medium" at best.
- Plate of food: identify each distinct component separately, estimate each one's portion weight from visual cues (use a ~27cm dinner plate as your size reference when one is visible), and usda_search each. Set confidence honestly per item -- a clearly identifiable component can be "medium"/"high", a hard-to-judge one should be "low".

FINISHING
- notes is a glance-line for Jim, not a report: at most one short sentence, and only for something he couldn't guess himself (an unusual portion assumption, ambiguous wording, a failed lookup). When the read was straightforward, return an empty string. Never re-list the items or narrate your process.
- Never write prose around tool calls. A response that calls a tool -- including record_meal/log_exercise -- must contain ONLY the tool call(s), no text before or after. Nobody reads that text; every token of it just makes the parse slower.
- Call exactly one of record_meal or log_exercise, as your final action, to submit the parse.`, day, docs.DietFramework, webGuidance)
}
