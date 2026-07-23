package nutrition

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFDCSearchUnitConversion(t *testing.T) {
	fixture, err := os.ReadFile("testdata/fdc_search_salmon.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	orig := fdcSearchURL
	fdcSearchURL = srv.URL
	defer func() { fdcSearchURL = orig }()

	c := NewFDCClient("DEMO_KEY")
	c.httpClient = srv.Client()

	foods, err := c.Search(context.Background(), "salmon", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(foods) == 0 {
		t.Fatal("expected at least one food result")
	}

	f := foods[0]
	if f.FDCID != 2016166 {
		t.Errorf("FDCID = %d, want 2016166", f.FDCID)
	}
	if f.DataType != "Branded" {
		t.Errorf("DataType = %q, want Branded", f.DataType)
	}
	if f.Brand == "" {
		t.Error("expected a non-empty brand for a Branded food")
	}
	// Source fixture: nutrientId 1008 (Energy) = 139 kcal, nutrientId 1003
	// (Protein) = 15.2g -> 15200mg, nutrientId 1093 (Sodium) = 331 (already mg).
	if f.Per100g.Calories != 139 {
		t.Errorf("Calories = %d, want 139", f.Per100g.Calories)
	}
	if f.Per100g.ProteinMg != 15200 {
		t.Errorf("ProteinMg = %d, want 15200", f.Per100g.ProteinMg)
	}
	if f.Per100g.SodiumMg != 331 {
		t.Errorf("SodiumMg = %d, want 331 (already mg, not converted again)", f.Per100g.SodiumMg)
	}
	if f.Per100g.FatMg != 7950 {
		t.Errorf("FatMg = %d, want 7950", f.Per100g.FatMg)
	}
	if len(f.Nutrients) == 0 {
		t.Error("expected raw nutrients to be kept for micros")
	}
	// The Survey (FNDDS) tier must be requested (item 3) so composite/prepared
	// dishes resolve instead of forcing an LLM estimate.
	if !strings.Contains(string(gotBody), "Survey (FNDDS)") {
		t.Errorf("request body did not ask for the Survey (FNDDS) data type: %s", gotBody)
	}
}
