// Package nutrition wraps USDA FoodData Central and Open Food Facts, the two
// grounded nutrition sources the AI parser calls as tools. Both clients
// return per-100g integer amounts (kcal, mg) so callers never touch floats.
package nutrition

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"time"
)

// fdcSearchURL is a var (not const) so tests can point it at an
// httptest.Server.
var fdcSearchURL = "https://api.nal.usda.gov/fdc/v1/foods/search"

// FDC nutrient IDs for the core amounts this app tracks.
const (
	nutrientEnergy  = 1008
	nutrientProtein = 1003
	nutrientFat     = 1004
	nutrientCarbs   = 1005
	nutrientFiber   = 1079
	nutrientSatFat  = 1258
	nutrientSugar   = 2000
	nutrientSodium  = 1093 // already reported in mg by FDC
)

// FDCClient searches USDA FoodData Central.
type FDCClient struct {
	apiKey     string
	httpClient *http.Client
}

func NewFDCClient(apiKey string) *FDCClient {
	return &FDCClient{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// PerHundredGrams is the core nutrition profile per 100g of a food, in the
// app's integer units.
type PerHundredGrams struct {
	Calories  int64 `json:"calories"`
	ProteinMg int64 `json:"protein_mg"`
	CarbsMg   int64 `json:"carbs_mg"`
	FatMg     int64 `json:"fat_mg"`
	FiberMg   int64 `json:"fiber_mg"`
	SatFatMg  int64 `json:"sat_fat_mg"`
	SugarMg   int64 `json:"sugar_mg"`
	SodiumMg  int64 `json:"sodium_mg"`
}

// NutrientAmount is one raw nutrient reading, kept alongside the core amounts
// so callers can surface extra micros beyond the eight this app tracks
// directly.
type NutrientAmount struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// FoodPortion is a household-measure gram weight from FDC (e.g. "1 cup" ->
// 140 g). Foundation/SR Legacy/Survey foods carry these; resolving a portion
// to grams from this list (item 3) is what fixes the quarter-cup vs half-cup
// class of error instead of letting the model guess grams.
type FoodPortion struct {
	Description string `json:"description"` // e.g. "1 cup", "1 medium"
	GramWeight  int    `json:"gram_weight"`
}

// FDCFood is one FoodData Central search result, per 100g.
type FDCFood struct {
	FDCID           int              `json:"fdc_id"`
	Description     string           `json:"description"`
	DataType        string           `json:"data_type"`
	Brand           string           `json:"brand,omitempty"`
	ServingSize     float64          `json:"serving_size,omitempty"`
	ServingSizeUnit string           `json:"serving_size_unit,omitempty"`
	Per100g         PerHundredGrams  `json:"per_100g"`
	Portions        []FoodPortion    `json:"portions,omitempty"`
	Nutrients       []NutrientAmount `json:"nutrients,omitempty"`
}

type fdcSearchRequest struct {
	Query    string   `json:"query"`
	PageSize int      `json:"pageSize"`
	DataType []string `json:"dataType"`
}

type fdcSearchResponse struct {
	Foods []fdcSearchFood `json:"foods"`
}

type fdcSearchFood struct {
	FDCID           int               `json:"fdcId"`
	Description     string            `json:"description"`
	DataType        string            `json:"dataType"`
	BrandOwner      string            `json:"brandOwner"`
	BrandName       string            `json:"brandName"`
	ServingSize     float64           `json:"servingSize"`
	ServingSizeUnit string            `json:"servingSizeUnit"`
	FoodNutrients   []fdcFoodNutrient `json:"foodNutrients"`
	FoodMeasures    []fdcFoodMeasure  `json:"foodMeasures"`
}

// fdcFoodMeasure is one household-measure entry from the search response.
// disseminationText is the human label ("1 cup"); gramWeight is its mass.
type fdcFoodMeasure struct {
	DisseminationText string  `json:"disseminationText"`
	Modifier          string  `json:"modifier"`
	GramWeight        float64 `json:"gramWeight"`
}

type fdcFoodNutrient struct {
	NutrientID   int     `json:"nutrientId"`
	NutrientName string  `json:"nutrientName"`
	UnitName     string  `json:"unitName"`
	Value        float64 `json:"value"`
}

// Search looks up foods by free-text query, preferring Foundation and SR
// Legacy data over Branded (matched by USDA's own relevance ranking, not
// re-sorted here). pageSize <= 0 defaults to 3, capped at 10 -- these results
// are fed to the AI parser as tokens, so small pages keep parses fast.
func (c *FDCClient) Search(ctx context.Context, query string, pageSize int) ([]FDCFood, error) {
	if pageSize <= 0 {
		pageSize = 3
	}
	if pageSize > 10 {
		pageSize = 10
	}
	// Data types in cascade order (item 3): Foundation + SR Legacy are the
	// analytically rigorous whole-food tiers, Survey (FNDDS) covers composite /
	// prepared / restaurant dishes ("salad bar", "fish tacos"), Branded is the
	// packaged fallback. USDA's own relevance ranking orders the results.
	reqBody, err := json.Marshal(fdcSearchRequest{
		Query:    query,
		PageSize: pageSize,
		DataType: []string{"Foundation", "SR Legacy", "Survey (FNDDS)", "Branded"},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal fdc request: %w", err)
	}

	url := fdcSearchURL + "?api_key=" + c.apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("create fdc request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fdc search: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read fdc response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fdc search failed with status %d: %s", resp.StatusCode, string(body))
	}

	var parsed fdcSearchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal fdc response: %w", err)
	}

	foods := make([]FDCFood, 0, len(parsed.Foods))
	for _, f := range parsed.Foods {
		food := FDCFood{
			FDCID:           f.FDCID,
			Description:     f.Description,
			DataType:        f.DataType,
			ServingSize:     f.ServingSize,
			ServingSizeUnit: f.ServingSizeUnit,
		}
		if f.DataType == "Branded" {
			if f.BrandName != "" {
				food.Brand = f.BrandName
			} else {
				food.Brand = f.BrandOwner
			}
		}
		for _, n := range f.FoodNutrients {
			switch n.NutrientID {
			case nutrientEnergy:
				food.Per100g.Calories = roundInt(n.Value)
			case nutrientProtein:
				food.Per100g.ProteinMg = gramsToMg(n.Value)
			case nutrientCarbs:
				food.Per100g.CarbsMg = gramsToMg(n.Value)
			case nutrientFat:
				food.Per100g.FatMg = gramsToMg(n.Value)
			case nutrientFiber:
				food.Per100g.FiberMg = gramsToMg(n.Value)
			case nutrientSatFat:
				food.Per100g.SatFatMg = gramsToMg(n.Value)
			case nutrientSugar:
				food.Per100g.SugarMg = gramsToMg(n.Value)
			case nutrientSodium:
				food.Per100g.SodiumMg = roundInt(n.Value)
			}
			food.Nutrients = append(food.Nutrients, NutrientAmount{
				Name: n.NutrientName, Value: n.Value, Unit: n.UnitName,
			})
		}
		for _, m := range f.FoodMeasures {
			if m.GramWeight <= 0 {
				continue
			}
			desc := m.DisseminationText
			if desc == "" {
				desc = m.Modifier
			}
			food.Portions = append(food.Portions, FoodPortion{
				Description: desc, GramWeight: roundIntToInt(m.GramWeight),
			})
		}
		foods = append(foods, food)
	}
	return foods, nil
}

func gramsToMg(grams float64) int64 {
	return roundInt(grams * 1000)
}

func roundInt(v float64) int64 {
	return int64(math.Round(v))
}

func roundIntToInt(v float64) int {
	return int(math.Round(v))
}
