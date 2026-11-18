package earth

import (
	"net/url"
	"strings"
	"testing"

	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

func TestTerrainTilesHaveOneCompleteZoom(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	wategos := [4]float64{153.61, -28.65, 153.66, -28.61}
	for source, prefix := range map[string]string{
		"terrain-tiles":          "14-",
		"terrain-tiles-offshore": "10-",
	} {
		images, err := Find(source, get, Query{Bounds: &wategos, Most: 50})
		if err != nil || len(images) == 0 {
			t.Fatalf("%s returned %v, %v", source, images, err)
		}
		for _, image := range images {
			if !strings.HasPrefix(image.Name(), prefix) ||
				!strings.HasPrefix(image.URL(), terrainBucket+"/") {
				t.Fatalf("unexpected tile: %v", image.URL())
			}
		}
	}
	images, err := Find("terrain-tiles", get, Query{Bounds: &wategos, Most: 1})
	if err != nil || len(images) != 1 ||
		strings.HasPrefix(images[0].Name(), "14-") {
		t.Fatalf("oversized query did not choose a lower zoom: %v %v",
			images, err)
	}
}

func TestETOPOPreservesAxisOrder(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	wategos := [4]float64{153.6, -28.66, 153.7, -28.60}
	products, err := Find("noaa-etopo", get, Query{Bounds: &wategos})
	if err != nil || len(products) != 1 {
		t.Fatalf("ETOPO discovery: %v, %v", products, err)
	}
	address, err := url.Parse(products[0].URL())
	if err != nil {
		t.Fatal(err)
	}
	query, err := url.QueryUnescape(address.RawQuery)
	if err != nil || query != "z[(-28.66):1:(-28.6)][(153.6):1:(153.7)]" {
		t.Fatalf("wrong axis order in %q: %v", query, err)
	}
	wategos[2] += 2
	if _, err := Find("noaa-etopo", get, Query{Bounds: &wategos}); err == nil {
		t.Fatal("accepted an oversized request")
	}
}
