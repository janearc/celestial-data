package earth

import (
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

func TestWaterQualitySnapshots(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	products, err := Find("usgs-water-quality", get, Query{Bounds: &box})
	if err != nil || len(products) != 5 {
		t.Fatalf("got %d products, error %v", len(products), err)
	}
	for index, code := range []string{"63680", "90860", "00480", "80154", "80300"} {
		product := products[index]
		where, err := url.Parse(product.URL())
		if err != nil || product.ID() != code ||
			where.Query().Get("parameter_code") != code ||
			where.Query().Get("bbox") != "-123,37,-122,38" ||
			where.Query().Get("limit") != "1000" ||
			where.Query().Get("f") != "json" {
			t.Fatalf("wrong USGS product %q: %v", code, product)
		}
		if !strings.Contains(product.About(), "dates and pagination") {
			t.Fatalf("missing snapshot limitation: %s", product.About())
		}
	}
	limited, err := Find("usgs-water-quality", get, Query{Bounds: &box, Most: 2})
	if err != nil || len(limited) != 2 {
		t.Fatalf("expected two products: %v, %v", limited, err)
	}
}

func TestWaterQualityParameterChoice(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	for text, code := range map[string]string{
		"salinity": "90860", " SALINITY-PPT ": "00480",
		"estimated-sediment": "80300", "00010": "00010",
	} {
		products, err := Find("usgs-water-quality", get, Query{Bounds: &box, Text: text})
		if err != nil || len(products) != 1 || products[0].ID() != code {
			t.Fatalf("parameter %q: %v, %v", text, products, err)
		}
	}
	for _, bad := range []string{"0001", "12/45", "turbid"} {
		_, err := Find("usgs-water-quality", get, Query{Bounds: &box, Text: bad})
		if !errors.Is(err, celestialdata.Refused()) {
			t.Fatalf("invalid parameter %q: %v", bad, err)
		}
	}
}
