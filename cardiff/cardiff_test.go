package cardiff_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/cardiff"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// results is the folder that holds each tile's folder.
const results = "http://www.astro.cardiff.ac.uk/research/ViaLactea/" +
	"PPMAP_Results/"

// The counts and places the tests work with.
const (
	tiles   = 3 // defined: l358, l000 and l002 lie within the slack
	maps    = 4 // defined: each tile's folder holds four maps
	first   = 0 // defined: the first product
	unsized = 0 // defined: the size of a file the archive does not size
)

// The north galactic pole, far from every tile on the plane. It's
// defined by the Hipparcos definition of the galactic frame.
const (
	poleRA  = 192.85948 // defined: the pole's right ascension
	poleDec = 27.12825  // defined: the pole's declination
	wide    = 0.1       // defined: the radius of the circle there
)

// resultsOf is the results asked through get, with the l000 tile's size.
func resultsOf(t *testing.T, get *mocks.MockWeb) *cardiff.Results {
	t.Helper()
	tile, err := cardiff.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	found, err := cardiff.New(get, tile)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// tileAt is the address of a tile's folder.
func tileAt(name string) string { return results + name + "_results/" }

// Sgr A* is near the l000 tile's middle. The slack lets the tiles on
// either side be found too, and each brings its four maps.
func TestFindNamesTheMapsOfTheTilesNearby(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "results.html",
		tileAt("l000"): "l000.html", tileAt("l002"): "l002.html",
		tileAt("l358"): "l358.html"})
	found := resultsOf(t, get)
	products, err := found.Find(replay.Asked(t))
	if err != nil || len(products) != tiles*maps {
		t.Fatalf("%d products, %v", len(products), err)
	}
	want := tileAt("l000") + "l000_cdens.fits"
	for _, product := range products {
		if product.URL() != tileAt(product.ID())+product.Name() ||
			product.Archive() != found.Name() ||
			product.About() != "Hi-GAL PPMAP, tile "+product.ID() {
			t.Errorf("%s %s %q", product.ID(), product.URL(),
				product.About())
		}
		if product.URL() == want {
			want = ""
		}
	}
	if want != "" {
		t.Errorf("no %s", want)
	}
}

// Circles at the galactic pole reach no tile. No tile is asked.
func TestFindAsksNoTileFarFromThePlane(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "results.html"})
	centre, err := celestialdata.NewPosition(poleRA, poleDec)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(centre, wide)
	if err != nil {
		t.Fatal(err)
	}
	if products, err := resultsOf(t, get).Find(far); products != nil ||
		err != nil {
		t.Errorf("%v, %v", products, err)
	}
}

// Links out of a tile's folder are not its maps.
func TestFindKeepsOnlyTheTilesFiles(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "one-tile.html",
		tileAt("l000"): "leading-out.html"})
	products, err := resultsOf(t, get).Find(replay.Asked(t))
	if err != nil || len(products) != len("1") ||
		products[first].Name() != "l000_temp.fits" {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-test: a tile whose longitude will not read is an error that names
// it.
func TestFindReportsATileThatWillNotRead(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "crooked-tile.html"})
	_, err := resultsOf(t, get).Find(replay.Asked(t))
	if err == nil || !strings.Contains(err.Error(), "l9999") {
		t.Errorf("%v", err)
	}
}

// Anti-test: results that will not list are an error that says why.
func TestFindReportsResultsThatWillNotList(t *testing.T) {
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(results).Return(nil, int64(unsized), down)
	if _, err := resultsOf(t, get).Find(replay.Asked(t)); !errors.Is(err,
		down) {
		t.Errorf("%v", err)
	}
}

// Anti-test: a tile that will not list is an error that says why.
func TestFindReportsATileThatWillNotList(t *testing.T) {
	down := errors.New("down")
	get := replay.Web(t, map[string]string{results: "one-tile.html"})
	get.EXPECT().Get(tileAt("l000")).Return(nil, int64(unsized), down)
	if _, err := resultsOf(t, get).Find(replay.Asked(t)); !errors.Is(err,
		down) {
		t.Errorf("%v", err)
	}
}

// Anti-tests: New refuses no web and no tile.
func TestNewRefusesWhatCannotFind(t *testing.T) {
	tile, err := cardiff.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	get := mocks.NewMockWeb(gomock.NewController(t))
	if _, err := cardiff.New(nil, tile); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
	if _, err := cardiff.New(get, nil); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no tile: %v", err)
	}
}

// The files found come whole: each is of the size the adapter gave it.
func TestFoundFilesComeWhole(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "one-tile.html",
		tileAt("l000"): "l000.html"})
	products, err := resultsOf(t, get).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, products[first]); err != nil {
		t.Errorf("%v", err)
	}
}

// quarter is a quarter turn, in degrees. It's defined.
const quarter = 90

// The slack is a degree. The ascending node is at l 32.93 by the galactic
// frame's definition. There the circle reaches l033 and l035. It does
// not reach l030, nearly three degrees away.
func TestTheSlackIsADegree(t *testing.T) {
	get := replay.Web(t, map[string]string{results: "results.html",
		tileAt("l033"): "l033.html", tileAt("l035"): "l035.html"})
	centre, err := celestialdata.NewPosition(poleRA+quarter, equator)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, wide)
	if err != nil {
		t.Fatal(err)
	}
	products, err := resultsOf(t, get).Find(near)
	if err != nil || len(products) != maps+maps {
		t.Errorf("%d products, %v", len(products), err)
	}
}

// equator is the declination of the celestial equator. It's defined.
const equator = 0
