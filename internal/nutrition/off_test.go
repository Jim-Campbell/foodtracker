package nutrition

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestOFFLookupUnitConversion(t *testing.T) {
	fixture, err := os.ReadFile("testdata/off_barcode_0016000275270.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != offUserAgent {
			t.Errorf("User-Agent = %q, want %q", got, offUserAgent)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(fixture)
	}))
	defer srv.Close()

	orig := offBaseURL
	offBaseURL = srv.URL
	defer func() { offBaseURL = orig }()

	c := NewOFFClient()
	c.httpClient = srv.Client()

	p, err := c.Lookup(context.Background(), "0016000275270")
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}

	// Source fixture: energy-kcal_100g=393 (already kcal, not the 1712.19 kJ
	// energy_100g value), proteins_100g=7.14g -> 7140mg, sodium_100g=0.571g
	// -> 571mg.
	if p.Per100g.Calories != 393 {
		t.Errorf("Calories = %d, want 393 (from energy-kcal_100g, not energy_100g kJ)", p.Per100g.Calories)
	}
	if p.Per100g.ProteinMg != 7140 {
		t.Errorf("ProteinMg = %d, want 7140", p.Per100g.ProteinMg)
	}
	if p.Per100g.SodiumMg != 571 {
		t.Errorf("SodiumMg = %d, want 571", p.Per100g.SodiumMg)
	}
	if p.Per100g.SugarMg != 32140 {
		t.Errorf("SugarMg = %d, want 32140", p.Per100g.SugarMg)
	}
	if p.PerServing == nil {
		t.Fatal("expected per-serving nutriments to be present")
	}
	if p.PerServing.Calories != 110 {
		t.Errorf("PerServing.Calories = %d, want 110", p.PerServing.Calories)
	}
}

func TestOFFLookupNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":0,"code":"0000000000000"}`))
	}))
	defer srv.Close()

	orig := offBaseURL
	offBaseURL = srv.URL
	defer func() { offBaseURL = orig }()

	c := NewOFFClient()
	c.httpClient = srv.Client()

	if _, err := c.Lookup(context.Background(), "0000000000000"); err == nil {
		t.Error("expected an error for an unknown barcode, got nil")
	}
}
