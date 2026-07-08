package nutrition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

const offUserAgent = "food-app/1.0 (personal tracker)"

// offBaseURL is a var (not const) so tests can point it at an
// httptest.Server.
var offBaseURL = "https://world.openfoodfacts.org"

// OFFClient looks up products by barcode via Open Food Facts.
type OFFClient struct {
	httpClient *http.Client
}

func NewOFFClient() *OFFClient {
	return &OFFClient{httpClient: &http.Client{Timeout: 10 * time.Second}}
}

// Product is one Open Food Facts lookup result. PerServing is nil when the
// product has no serving-size nutriments.
type Product struct {
	Name        string           `json:"name"`
	Brands      string           `json:"brands,omitempty"`
	ServingSize string           `json:"serving_size,omitempty"`
	Per100g     PerHundredGrams  `json:"per_100g"`
	PerServing  *PerHundredGrams `json:"per_serving,omitempty"`
}

type offResponse struct {
	Status  int        `json:"status"`
	Product offProduct `json:"product"`
}

type offProduct struct {
	ProductName string        `json:"product_name"`
	Brands      string        `json:"brands"`
	ServingSize string        `json:"serving_size"`
	Nutriments  offNutriments `json:"nutriments"`
}

// offNutriments maps the subset of Open Food Facts' nutriments object this
// app uses. energy-kcal_100g is already kcal -- energy_100g is kJ and must
// never be used for calories.
type offNutriments struct {
	EnergyKcal100g       *float64 `json:"energy-kcal_100g"`
	EnergyKcalServing    *float64 `json:"energy-kcal_serving"`
	Proteins100g         *float64 `json:"proteins_100g"`
	ProteinsServing      *float64 `json:"proteins_serving"`
	Carbohydrates100g    *float64 `json:"carbohydrates_100g"`
	CarbohydratesServing *float64 `json:"carbohydrates_serving"`
	Fat100g              *float64 `json:"fat_100g"`
	FatServing           *float64 `json:"fat_serving"`
	Fiber100g            *float64 `json:"fiber_100g"`
	FiberServing         *float64 `json:"fiber_serving"`
	SaturatedFat100g     *float64 `json:"saturated-fat_100g"`
	SaturatedFatServing  *float64 `json:"saturated-fat_serving"`
	Sugars100g           *float64 `json:"sugars_100g"`
	SugarsServing        *float64 `json:"sugars_serving"`
	Sodium100g           *float64 `json:"sodium_100g"`
	SodiumServing        *float64 `json:"sodium_serving"`
}

// Lookup fetches a product by barcode. Returns an error (never nil, never a
// crash) when the barcode isn't found, so the AI tool layer can fall back to
// estimating.
func (c *OFFClient) Lookup(ctx context.Context, barcode string) (*Product, error) {
	reqURL := fmt.Sprintf("%s/api/v2/product/%s.json", offBaseURL, url.PathEscape(barcode))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create off request: %w", err)
	}
	req.Header.Set("User-Agent", offUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("off lookup: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read off response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("off lookup failed with status %d: %s", resp.StatusCode, string(body))
	}

	var parsed offResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("unmarshal off response: %w", err)
	}
	if parsed.Status == 0 {
		return nil, fmt.Errorf("barcode %s not found in Open Food Facts", barcode)
	}

	n := parsed.Product.Nutriments
	product := &Product{
		Name:        parsed.Product.ProductName,
		Brands:      parsed.Product.Brands,
		ServingSize: parsed.Product.ServingSize,
		Per100g: PerHundredGrams{
			Calories:  roundInt(deref(n.EnergyKcal100g)),
			ProteinMg: gramsToMg(deref(n.Proteins100g)),
			CarbsMg:   gramsToMg(deref(n.Carbohydrates100g)),
			FatMg:     gramsToMg(deref(n.Fat100g)),
			FiberMg:   gramsToMg(deref(n.Fiber100g)),
			SatFatMg:  gramsToMg(deref(n.SaturatedFat100g)),
			SugarMg:   gramsToMg(deref(n.Sugars100g)),
			SodiumMg:  gramsToMg(deref(n.Sodium100g)),
		},
	}
	if n.EnergyKcalServing != nil {
		product.PerServing = &PerHundredGrams{
			Calories:  roundInt(deref(n.EnergyKcalServing)),
			ProteinMg: gramsToMg(deref(n.ProteinsServing)),
			CarbsMg:   gramsToMg(deref(n.CarbohydratesServing)),
			FatMg:     gramsToMg(deref(n.FatServing)),
			FiberMg:   gramsToMg(deref(n.FiberServing)),
			SatFatMg:  gramsToMg(deref(n.SaturatedFatServing)),
			SugarMg:   gramsToMg(deref(n.SugarsServing)),
			SodiumMg:  gramsToMg(deref(n.SodiumServing)),
		}
	}
	return product, nil
}

func deref(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
