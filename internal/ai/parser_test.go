package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/jimgcampbell/food/internal/food"
	"github.com/jimgcampbell/food/internal/nutrition"
)

// fakeMessenger scripts a sequence of Messages API responses so the loop
// mechanics can be tested without any HTTP transport.
type fakeMessenger struct {
	responses []*Response
	calls     []Request
}

func (f *fakeMessenger) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	f.calls = append(f.calls, req)
	if len(f.responses) == 0 {
		return nil, fmt.Errorf("fakeMessenger: no more scripted responses (call %d)", len(f.calls))
	}
	resp := f.responses[0]
	f.responses = f.responses[1:]
	return resp, nil
}

type fakeUSDA struct {
	called  bool
	results []nutrition.FDCFood
}

func (f *fakeUSDA) Search(ctx context.Context, query string, pageSize int) ([]nutrition.FDCFood, error) {
	f.called = true
	return f.results, nil
}

type fakeOFF struct {
	called bool
}

func (f *fakeOFF) Lookup(ctx context.Context, barcode string) (*nutrition.Product, error) {
	f.called = true
	return &nutrition.Product{Name: "test product"}, nil
}

type fakeCanonical struct {
	entry  *food.CanonicalFood
	called bool
}

func (f *fakeCanonical) LookupCanonical(ctx context.Context, normalizedName string) (*food.CanonicalFood, error) {
	f.called = true
	return f.entry, nil
}

func toolUseBlock(id, name string, input any) json.RawMessage {
	body, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	raw, err := json.Marshal(struct {
		Type  string          `json:"type"`
		ID    string          `json:"id"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}{"tool_use", id, name, body})
	if err != nil {
		panic(err)
	}
	return raw
}

// thinkingBlock simulates an extended-thinking block the way Claude actually
// sends one: fields (thinking, signature) this app doesn't model at all.
// blockText distinguishes one fixture from another in assertions.
func thinkingBlock(blockText string) json.RawMessage {
	raw, err := json.Marshal(map[string]string{
		"type": "thinking", "thinking": blockText, "signature": "sig-" + blockText,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

func decodeBlockType(t *testing.T, raw json.RawMessage) string {
	t.Helper()
	var v struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		t.Fatalf("decode block type: %v", err)
	}
	return v.Type
}

func validItem(overrides func(*food.MealItem)) food.MealItem {
	it := food.MealItem{
		Name: "Egg", Quantity: "1 large", FractionPct: 100,
		Calories: 70, ProteinMg: 6000, CarbsMg: 0, FatMg: 5000, FiberMg: 0,
		SatFatMg: 1500, SugarMg: 0, SodiumMg: 70,
		Tier: food.TierHardYes, TierReason: "eggs are hard yes",
		Source: food.SourceUSDA, Confidence: food.ConfidenceHigh,
	}
	if overrides != nil {
		overrides(&it)
	}
	return it
}

func TestParserToolLoopMechanics(t *testing.T) {
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "egg", "page_size": 5})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(nil)},
				Notes: "assumed one large egg",
			})},
		},
	}}
	usda := &fakeUSDA{results: []nutrition.FDCFood{{FDCID: 1, Description: "Egg, whole, raw"}}}
	off := &fakeOFF{}

	p := NewParser(messenger, usda, off, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "an egg", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	if !usda.called {
		t.Error("expected usda_search to have been executed")
	}
	if off.called {
		t.Error("off_barcode should not have been called")
	}
	if len(messenger.calls) != 2 {
		t.Errorf("CreateMessage called %d times, want 2", len(messenger.calls))
	}
	if result.AIModel != "claude-sonnet-5" {
		t.Errorf("AIModel = %q, want claude-sonnet-5", result.AIModel)
	}
	if len(result.Items) != 1 || result.Items[0].Name != "Egg" {
		t.Fatalf("Items = %+v, want one item named Egg", result.Items)
	}
	if !strings.Contains(result.Notes, "assumed one large egg") {
		t.Errorf("Notes = %q, want it to contain the model's stated assumption", result.Notes)
	}
	if len(result.AIRaw) == 0 {
		t.Error("expected AIRaw to hold the full trace")
	}

	// The second call should carry the tool_result from the first round.
	secondCallMsgs := messenger.calls[1].Messages
	last := secondCallMsgs[len(secondCallMsgs)-1]
	if last.Role != RoleUser || len(last.Content) == 0 || decodeBlockType(t, last.Content[0]) != "tool_result" {
		t.Errorf("expected the round-2 request to end with a tool_result message, got %+v", last)
	}
}

// TestParserMicrosAttachedServerSide: the model gets slim usda_search results
// (no per-nutrient list -- that's the token bloat that made parses slow) while
// finish() still attaches the full cached nutrient payload as micros on items
// whose source/source_ref match a lookup from this parse.
func TestParserMicrosAttachedServerSide(t *testing.T) {
	ref := "173735"
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "egg"})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) { it.SourceRef = &ref })},
			})},
		},
	}}
	usda := &fakeUSDA{results: []nutrition.FDCFood{{
		FDCID: 173735, Description: "Egg, whole, raw",
		Nutrients: []nutrition.NutrientAmount{
			{Name: "Vitamin B-12", Value: 0.89, Unit: "UG"},
			{Name: "Choline, total", Value: 293.8, Unit: "MG"},
		},
	}}}

	p := NewParser(messenger, usda, &fakeOFF{}, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "an egg", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	// The round-2 request carries the tool_result; it must not include the
	// nutrient list the model has no use for.
	secondCallMsgs := messenger.calls[1].Messages
	last := secondCallMsgs[len(secondCallMsgs)-1]
	var tr struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(last.Content[0], &tr); err != nil {
		t.Fatalf("decode tool_result: %v", err)
	}
	if strings.Contains(tr.Content, "Vitamin B-12") {
		t.Errorf("tool_result sent to the model still contains the full nutrient list: %s", tr.Content)
	}
	if !strings.Contains(tr.Content, "Egg, whole, raw") {
		t.Errorf("tool_result lost the food description: %s", tr.Content)
	}

	// ...while the returned item carries the full payload as micros.
	if !strings.Contains(string(result.Items[0].Micros), "Vitamin B-12") {
		t.Errorf("Micros = %s, want the cached full nutrient payload attached server-side", result.Items[0].Micros)
	}
}

// TestParserResolvesNutritionFromFDC: for a usda item the app computes every
// macro from the cached FDC per-100g times the resolved grams (item 3) and
// discards whatever numbers the model put in the tool call, and it records the
// resolution provenance.
func TestParserResolvesNutritionFromFDC(t *testing.T) {
	ref := "2016166"
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "salmon"})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) {
					it.Name = "salmon"
					it.Source = food.SourceUSDA
					it.SourceRef = &ref
					dt := "Foundation"
					it.FDCDataType = &dt
					g := 150
					it.Grams = &g
					// Deliberately wrong model numbers -- the app must overwrite them.
					it.Calories = 9999
					it.ProteinMg = 1
				})},
			})},
		},
	}}
	usda := &fakeUSDA{results: []nutrition.FDCFood{{
		FDCID: 2016166, Description: "Salmon", DataType: "Foundation",
		Per100g: nutrition.PerHundredGrams{Calories: 200, ProteinMg: 25000, FatMg: 12000, SatFatMg: 2000, SodiumMg: 60},
	}}}

	p := NewParser(messenger, usda, &fakeOFF{}, nil, "", false, slog.Default())
	result, err := p.ParseText(context.Background(), "150g salmon", "2026-07-20", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	it := result.Items[0]
	if it.Calories != 300 { // 200 per 100g * 150 g
		t.Errorf("Calories = %d, want 300 (FDC per-100g * grams), not the model's 9999", it.Calories)
	}
	if it.ProteinMg != 37500 {
		t.Errorf("ProteinMg = %d, want 37500 (25000 * 150 / 100)", it.ProteinMg)
	}
	if it.SatFatMg != 3000 {
		t.Errorf("SatFatMg = %d, want 3000", it.SatFatMg)
	}
	if it.FDCID == nil || *it.FDCID != 2016166 {
		t.Errorf("FDCID = %v, want 2016166", it.FDCID)
	}
	if it.ResolutionTier == nil || *it.ResolutionTier != food.ResolutionFoundationSR {
		t.Errorf("ResolutionTier = %v, want 2 (Foundation)", it.ResolutionTier)
	}
	if it.PortionSource == nil || *it.PortionSource != food.PortionEstimated {
		t.Errorf("PortionSource = %v, want estimated", it.PortionSource)
	}
	if it.TierSource == nil || *it.TierSource != food.TierSourceTable {
		t.Errorf("TierSource = %v, want the table default", it.TierSource)
	}
}

// TestParserAttachesMatchAlternatives: for a usda item, finish() sets the
// chosen FDC entry's name and turns the model's alternative_fdc_ids into full
// MatchAlternatives (with per-100g) for the one-tap swap (item 4).
func TestParserAttachesMatchAlternatives(t *testing.T) {
	chosen, alt1, alt2 := "111", "222", "333"
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "salmon"})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) {
					it.Name = "salmon"
					it.Source = food.SourceUSDA
					it.SourceRef = &chosen
					dt := "Foundation"
					it.FDCDataType = &dt
					g := 100
					it.Grams = &g
					it.AlternativeFDCIDs = []string{alt1, alt2}
				})},
			})},
		},
	}}
	usda := &fakeUSDA{results: []nutrition.FDCFood{
		{FDCID: 111, Description: "Salmon, Atlantic, cooked", DataType: "Foundation", Per100g: nutrition.PerHundredGrams{Calories: 200, ProteinMg: 25000}},
		{FDCID: 222, Description: "Salmon, pink, canned", DataType: "SR Legacy", Per100g: nutrition.PerHundredGrams{Calories: 140, ProteinMg: 20000}},
		{FDCID: 333, Description: "SALMON (Branded)", DataType: "Branded", Per100g: nutrition.PerHundredGrams{Calories: 210, ProteinMg: 22000}},
	}}

	p := NewParser(messenger, usda, &fakeOFF{}, nil, "", false, slog.Default())
	result, err := p.ParseText(context.Background(), "salmon", "2026-07-20", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	it := result.Items[0]
	if it.MatchDescription != "Salmon, Atlantic, cooked" {
		t.Errorf("MatchDescription = %q, want the chosen entry's name", it.MatchDescription)
	}
	if len(it.Alternatives) != 2 {
		t.Fatalf("Alternatives = %d, want 2", len(it.Alternatives))
	}
	if it.Alternatives[0].Description != "Salmon, pink, canned" || it.Alternatives[0].FDCID != 222 {
		t.Errorf("alt[0] = %+v, want the canned-salmon match", it.Alternatives[0])
	}
	if it.Alternatives[0].Per100g.Calories != 140 {
		t.Errorf("alt[0] per-100g calories = %d, want 140 (so a swap can recompute)", it.Alternatives[0].Per100g.Calories)
	}
	if it.AlternativeFDCIDs != nil {
		t.Errorf("AlternativeFDCIDs should be cleared, got %v", it.AlternativeFDCIDs)
	}
}

// TestParserCanonicalReuse: on a canonical_lookup hit the model records the
// item straight from the cached entry (no usda_search), and finish() computes
// nutrition from the cached per-100g exactly as it would from a fresh lookup
// (item 3 accretion).
func TestParserCanonicalReuse(t *testing.T) {
	ref, dt := "173735", "Foundation"
	canon := &fakeCanonical{entry: &food.CanonicalFood{
		NormalizedName: "orgain protein powder", DisplayName: "Orgain protein powder",
		Source: food.SourceUSDA, SourceRef: &ref, FDCDataType: &dt,
		Per100g: food.Per100{Calories: 400, ProteinMg: 80000},
		Tier:    food.TierSoftYes,
	}}
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolCanonicalLookup, map[string]any{"name": "Orgain protein powder"})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) {
					it.Name = "Orgain protein powder"
					it.Source = food.SourceUSDA
					it.SourceRef = &ref
					it.FDCDataType = &dt
					g := 25
					it.Grams = &g
					it.Calories = 9999 // must be overwritten from the cached per-100g
				})},
			})},
		},
	}}
	usda := &fakeUSDA{}

	p := NewParser(messenger, usda, &fakeOFF{}, canon, "", false, slog.Default())
	result, err := p.ParseText(context.Background(), "an orgain shake", "2026-07-20", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if !canon.called {
		t.Error("expected canonical_lookup to be called")
	}
	if usda.called {
		t.Error("usda_search should be skipped on a canonical hit")
	}
	it := result.Items[0]
	if it.Calories != 100 { // 400 per 100g * 25 g
		t.Errorf("Calories = %d, want 100 from the cached per-100g", it.Calories)
	}
	if it.ProteinMg != 20000 { // 80000 * 25 / 100
		t.Errorf("ProteinMg = %d, want 20000", it.ProteinMg)
	}
}

// TestParserPauseTurnResumes: a server tool (web search) can pause the turn;
// the loop must replay the conversation as-is -- appending the assistant
// content but no user message -- so the API resumes the search.
func TestParserPauseTurnResumes(t *testing.T) {
	serverToolBlock, _ := json.Marshal(map[string]any{
		"type": "server_tool_use", "id": "st1", "name": "web_search",
		"input": map[string]string{"query": "five guys cheeseburger nutrition"},
	})
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "pause_turn", Model: "claude-sonnet-5",
			Content: []json.RawMessage{serverToolBlock},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) { it.Source = food.SourceWeb })},
			})},
		},
	}}

	p := NewParser(messenger, &fakeUSDA{}, &fakeOFF{}, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "5 guys standard burger", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}
	if result.Items[0].Source != food.SourceWeb {
		t.Errorf("Source = %q, want web", result.Items[0].Source)
	}

	// First call must offer the web_search server tool.
	var hasWeb bool
	for _, tool := range messenger.calls[0].Tools {
		if tool.Type == "web_search_20250305" {
			hasWeb = true
		}
	}
	if !hasWeb {
		t.Error("first request did not include the web_search server tool")
	}

	// The resumed call replays [user, assistant] with no injected user text.
	second := messenger.calls[1].Messages
	if len(second) != 2 {
		t.Fatalf("resumed call has %d messages, want 2 (original user + paused assistant)", len(second))
	}
	if second[1].Role != RoleAssistant || string(second[1].Content[0]) != string(json.RawMessage(serverToolBlock)) {
		t.Errorf("paused assistant turn was not replayed byte-for-byte")
	}
}

// TestParserPreservesUnmodeledBlocks guards against the real bug this
// exposed live: Claude returned a "thinking" block this app has no typed
// field for. Round-tripping it through a lossy struct dropped the
// thinking/signature fields, and Anthropic's API rejected the mangled block
// on the next round ("messages.1.content.0.thinking.thinking: Field
// required"). Content must be kept as raw JSON end-to-end so blocks this app
// doesn't model survive being echoed back unchanged.
func TestParserPreservesUnmodeledBlocks(t *testing.T) {
	think := thinkingBlock("reasoning about the egg")
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{
				think,
				toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "egg"}),
			},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(nil)},
				Notes: "assumed one large egg",
			})},
		},
	}}

	p := NewParser(messenger, &fakeUSDA{}, &fakeOFF{}, nil, "", true, slog.Default())
	if _, err := p.ParseText(context.Background(), "an egg", "2026-07-07", nil); err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	secondCallMsgs := messenger.calls[1].Messages
	// messages[0] = original user text, [1] = assistant turn from round 1
	// (thinking block + tool_use), [2] = our tool_result reply.
	assistantTurn := secondCallMsgs[1]
	if assistantTurn.Role != RoleAssistant || len(assistantTurn.Content) == 0 {
		t.Fatalf("expected the replayed assistant turn, got %+v", assistantTurn)
	}
	if string(assistantTurn.Content[0]) != string(think) {
		t.Errorf("thinking block was not preserved byte-for-byte:\ngot:  %s\nwant: %s", assistantTurn.Content[0], think)
	}
}

func TestParserRoundCapForcesRecordMeal(t *testing.T) {
	var responses []*Response
	for i := 0; i < maxToolRounds; i++ {
		responses = append(responses, &Response{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock(fmt.Sprintf("t%d", i), toolUSDASearch, map[string]any{"query": "mystery food"})},
		})
	}
	responses = append(responses, &Response{
		StopReason: "tool_use", Model: "claude-sonnet-5",
		Content: []json.RawMessage{toolUseBlock("final", toolRecordMeal, recordMealInput{
			Items: []food.MealItem{validItem(func(it *food.MealItem) { it.Source = food.SourceAI; it.Confidence = food.ConfidenceLow })},
			Notes: "forced after exhausting lookups",
		})},
	})
	messenger := &fakeMessenger{responses: responses}
	usda := &fakeUSDA{}
	off := &fakeOFF{}

	p := NewParser(messenger, usda, off, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "some mystery food", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	wantCalls := maxToolRounds + 1
	if len(messenger.calls) != wantCalls {
		t.Fatalf("CreateMessage called %d times, want %d (round cap + forced final call)", len(messenger.calls), wantCalls)
	}
	forced := messenger.calls[wantCalls-1]
	if forced.ToolChoice == nil || forced.ToolChoice.Type != "any" {
		t.Errorf("forced final call ToolChoice = %+v, want a forced any-tool choice", forced.ToolChoice)
	}
	var forcedNames []string
	for _, tool := range forced.Tools {
		forcedNames = append(forcedNames, tool.Name)
	}
	if !slices.Contains(forcedNames, toolRecordMeal) || !slices.Contains(forcedNames, toolLogExercise) {
		t.Errorf("forced final call Tools = %v, want both record_meal and log_exercise offered", forcedNames)
	}
	if !strings.Contains(result.Notes, "forced after exhausting lookups") {
		t.Errorf("Notes = %q, want the forced-round record_meal notes", result.Notes)
	}
}

func TestParserImageMessageShape(t *testing.T) {
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(func(it *food.MealItem) { it.Source = food.SourceLabel })},
				Notes: "read straight off the label",
			})},
		},
	}}

	p := NewParser(messenger, &fakeUSDA{}, &fakeOFF{}, nil, "claude-sonnet-5", true, slog.Default())
	imageBytes := []byte("fake-png-bytes")
	result, err := p.ParseImage(context.Background(), imageBytes, "image/png", "I had half of this", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseImage: %v", err)
	}
	if result.Items[0].Source != food.SourceLabel {
		t.Errorf("Source = %q, want label", result.Items[0].Source)
	}

	if got := messenger.calls[0].Model; got != "claude-sonnet-5" {
		t.Errorf("photo parse request model = %q, want the vision model override", got)
	}

	firstMsg := messenger.calls[0].Messages[0]
	if firstMsg.Role != RoleUser {
		t.Fatalf("first message role = %q, want user", firstMsg.Role)
	}
	if len(firstMsg.Content) != 2 {
		t.Fatalf("first message has %d content blocks, want 2 (image + text)", len(firstMsg.Content))
	}
	if decodeBlockType(t, firstMsg.Content[0]) != "image" {
		t.Errorf("first block type = %q, want image", decodeBlockType(t, firstMsg.Content[0]))
	}
	var img struct {
		Source struct {
			Data string `json:"data"`
		} `json:"source"`
	}
	if err := json.Unmarshal(firstMsg.Content[0], &img); err != nil {
		t.Fatalf("decode image block: %v", err)
	}
	if got := base64.StdEncoding.EncodeToString(imageBytes); img.Source.Data != got {
		t.Errorf("image data = %q, want base64 of the original bytes %q", img.Source.Data, got)
	}
	if decodeBlockType(t, firstMsg.Content[1]) != "text" {
		t.Errorf("second block type = %q, want text", decodeBlockType(t, firstMsg.Content[1]))
	}
	var textBlock struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(firstMsg.Content[1], &textBlock); err != nil {
		t.Fatalf("decode text block: %v", err)
	}
	if !strings.Contains(textBlock.Text, "I had half of this") {
		t.Errorf("text block = %q, want it to contain the hint", textBlock.Text)
	}
}

func TestParserValidationWiring(t *testing.T) {
	// Claims 1000 kcal but only ~400 kcal of macros are present -- well
	// outside the +/-30% Atwater band, so this should clamp to low confidence
	// and add a warning to notes even though the item started at high.
	badItem := validItem(func(it *food.MealItem) {
		it.Calories = 1000
		it.ProteinMg = 0
		it.CarbsMg = 100000
		it.FatMg = 0
		it.Confidence = food.ConfidenceHigh
	})
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{badItem},
				Notes: "straightforward parse",
			})},
		},
	}}

	p := NewParser(messenger, &fakeUSDA{}, &fakeOFF{}, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "some carby thing", "2026-07-07", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	if len(messenger.calls) != 1 {
		t.Fatalf("CreateMessage called %d times, want 1 (record_meal on the first round)", len(messenger.calls))
	}
	if result.Items[0].Confidence != food.ConfidenceLow {
		t.Errorf("Confidence = %q, want low after an Atwater mismatch", result.Items[0].Confidence)
	}
	if !strings.Contains(result.Notes, "don't match") {
		t.Errorf("Notes = %q, want it to contain the Atwater mismatch warning", result.Notes)
	}
}

// TestParserLogExercise: a workout phrase should call log_exercise instead of
// record_meal, yielding a ParseResult with Kind "exercise" and no USDA
// lookups. Covers "hike then a swim" yielding two cardio sessions.
func TestParserLogExercise(t *testing.T) {
	dur := 30
	messenger := &fakeMessenger{responses: []*Response{
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []json.RawMessage{toolUseBlock("t1", toolLogExercise, logExerciseInput{
				Sessions: []food.ExerciseSession{
					{Type: food.ExerciseCardio, Activity: strPtr("Hike"), DurationMin: &dur},
					{Type: food.ExerciseCardio, Activity: strPtr("Swim"), DurationMin: &dur},
				},
			})},
		},
	}}
	usda := &fakeUSDA{}
	off := &fakeOFF{}

	p := NewParser(messenger, usda, off, nil, "", true, slog.Default())
	result, err := p.ParseText(context.Background(), "hike then a swim this morning", "2026-07-17", nil)
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	if result.Kind != food.ParseKindExercise {
		t.Errorf("Kind = %q, want %q", result.Kind, food.ParseKindExercise)
	}
	if len(result.Exercise) != 2 {
		t.Fatalf("Exercise = %+v, want 2 sessions", result.Exercise)
	}
	if len(result.Items) != 0 {
		t.Errorf("Items = %+v, want none for an exercise parse", result.Items)
	}
	if usda.called || off.called {
		t.Error("exercise parse should not call usda_search or off_barcode")
	}
	if len(result.AIRaw) == 0 {
		t.Error("expected AIRaw to hold the full trace")
	}
}

func strPtr(s string) *string { return &s }
