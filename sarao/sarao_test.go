package sarao_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/sarao"
	"go.uber.org/mock/gomock"
)

// The places the mosaic is asked for and kept.
const (
	// page is the DOI the adapter asks, which leads to the landing page.
	page = "https://doi.org/10.48479/fyst-hj47"
	// data is where the landing page recorded that night kept the files.
	data = "https://archive-gw-1.kat.ac.za/public/repository/10.48479/" +
		"fyst-hj47/data/"
)

// The counts and places the tests work with.
const (
	files   = 3   // defined: the FITS files the recorded page links
	first   = 0   // defined: the first product
	unsized = 0   // defined: the size of a file the archive does not size
	north   = 60  // defined: a declination far from the Galactic Centre
	east    = 0   // defined: a right ascension far from it too
	wide    = 0.1 // defined: the radius of the circle far away
)

// mosaicOf is the mosaic asked through get, over its own field.
func mosaicOf(t *testing.T, get *mocks.MockWeb) *sarao.Mosaic {
	t.Helper()
	field, err := sarao.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	mosaic, err := sarao.New(get, field)
	if err != nil {
		t.Fatal(err)
	}
	return mosaic
}

// Sgr A* is in the mosaic's field. So each file the page links is found
// with the licence and the citation the page asks for.
func TestFindNamesTheMosaicsFiles(t *testing.T) {
	get := replay.Web(t, map[string]string{page: "fyst-hj47.html"})
	mosaic := mosaicOf(t, get)
	products, err := mosaic.Find(replay.Asked(t))
	if err != nil || len(products) != files {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		if !strings.HasPrefix(product.Name(), "MeerKAT_Galactic_Centre_") ||
			product.URL() != data+product.Name() ||
			product.ID() != "10.48479/fyst-hj47" ||
			product.Archive() != mosaic.Name() ||
			!strings.Contains(product.About(), "CC BY-NC 4.0") ||
			!strings.Contains(product.About(), "arXiv:2201.10541") {
			t.Errorf("%s %s %q", product.Name(), product.URL(),
				product.About())
		}
	}
}

// Circles far from the field find nothing and ask nothing.
func TestFindAsksNothingFarFromTheField(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	mosaic := mosaicOf(t, get)
	centre, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(centre, wide)
	if err != nil {
		t.Fatal(err)
	}
	if products, err := mosaic.Find(far); products != nil || err != nil {
		t.Errorf("%v, %v", products, err)
	}
}

// Links elsewhere are not the data folder's. Nor are links below it,
// nor its files that are not FITS.
func TestFindKeepsOnlyTheDataFoldersFiles(t *testing.T) {
	get := replay.Web(t, map[string]string{page: "leading-out.html"})
	products, err := mosaicOf(t, get).Find(replay.Asked(t))
	if err != nil || len(products) != len("1") || products[first].Name() !=
		"MeerKAT_Galactic_Centre_alpha.fits" {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-tests: a page that will not come is an error, and New refuses no
// web and no field.
func TestWhatCannotBeAskedIsAnError(t *testing.T) {
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(page).Return(nil, int64(unsized), down)
	if _, err := mosaicOf(t, get).Find(replay.Asked(t)); !errors.Is(err,
		down) {
		t.Errorf("%v", err)
	}
	field, err := sarao.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := sarao.New(nil, field); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
	if _, err := sarao.New(get, nil); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no field: %v", err)
	}
}

// The files found come whole: each is of the size the adapter gave it.
func TestFoundFilesComeWhole(t *testing.T) {
	get := replay.Web(t, map[string]string{page: "fyst-hj47.html"})
	products, err := mosaicOf(t, get).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, products[first]); err != nil {
		t.Errorf("%v", err)
	}
}
