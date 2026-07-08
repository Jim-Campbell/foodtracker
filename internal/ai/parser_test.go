package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

func toolUseBlock(id, name string, input any) ContentBlock {
	body, err := json.Marshal(input)
	if err != nil {
		panic(err)
	}
	return ContentBlock{Type: "tool_use", ID: id, Name: name, Input: body}
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
			Content: []ContentBlock{toolUseBlock("t1", toolUSDASearch, map[string]any{"query": "egg", "page_size": 5})},
		},
		{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []ContentBlock{toolUseBlock("t2", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{validItem(nil)},
				Notes: "assumed one large egg",
			})},
		},
	}}
	usda := &fakeUSDA{results: []nutrition.FDCFood{{FDCID: 1, Description: "Egg, whole, raw"}}}
	off := &fakeOFF{}

	p := NewParser(messenger, usda, off, slog.Default())
	result, err := p.ParseText(context.Background(), "an egg", "2026-07-07")
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
	if last.Role != RoleUser || len(last.Content) == 0 || last.Content[0].Type != "tool_result" {
		t.Errorf("expected the round-2 request to end with a tool_result message, got %+v", last)
	}
}

func TestParserRoundCapForcesRecordMeal(t *testing.T) {
	var responses []*Response
	for i := 0; i < maxToolRounds; i++ {
		responses = append(responses, &Response{
			StopReason: "tool_use", Model: "claude-sonnet-5",
			Content: []ContentBlock{toolUseBlock(fmt.Sprintf("t%d", i), toolUSDASearch, map[string]any{"query": "mystery food"})},
		})
	}
	responses = append(responses, &Response{
		StopReason: "tool_use", Model: "claude-sonnet-5",
		Content: []ContentBlock{toolUseBlock("final", toolRecordMeal, recordMealInput{
			Items: []food.MealItem{validItem(func(it *food.MealItem) { it.Source = food.SourceAI; it.Confidence = food.ConfidenceLow })},
			Notes: "forced after exhausting lookups",
		})},
	})
	messenger := &fakeMessenger{responses: responses}
	usda := &fakeUSDA{}
	off := &fakeOFF{}

	p := NewParser(messenger, usda, off, slog.Default())
	result, err := p.ParseText(context.Background(), "some mystery food", "2026-07-07")
	if err != nil {
		t.Fatalf("ParseText: %v", err)
	}

	wantCalls := maxToolRounds + 1
	if len(messenger.calls) != wantCalls {
		t.Fatalf("CreateMessage called %d times, want %d (round cap + forced final call)", len(messenger.calls), wantCalls)
	}
	forced := messenger.calls[wantCalls-1]
	if forced.ToolChoice == nil || forced.ToolChoice.Type != "tool" || forced.ToolChoice.Name != toolRecordMeal {
		t.Errorf("forced final call ToolChoice = %+v, want a forced record_meal choice", forced.ToolChoice)
	}
	if !strings.Contains(result.Notes, "forced after exhausting lookups") {
		t.Errorf("Notes = %q, want the forced-round record_meal notes", result.Notes)
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
			Content: []ContentBlock{toolUseBlock("t1", toolRecordMeal, recordMealInput{
				Items: []food.MealItem{badItem},
				Notes: "straightforward parse",
			})},
		},
	}}

	p := NewParser(messenger, &fakeUSDA{}, &fakeOFF{}, slog.Default())
	result, err := p.ParseText(context.Background(), "some carby thing", "2026-07-07")
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
