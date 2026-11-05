package irsa_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/irsa"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// The counts the recorded listing gives for the circle round Sgr A*.
const (
	bands   = 4 // defined: IRAC's four bands, one mosaic each
	unsized = 0 // defined: the size of a file the archive does not size
	first   = 0 // defined: the first product
)

// glimpseOf is GLIMPSE II's folder asked through a web that answers with
// one recorded listing.
func glimpseOf(t *testing.T, file string) *irsa.Glimpse {
	t.Helper()
	get := replay.Web(t, map[string]string{irsa.GlimpseII: file})
	glimpse, err := irsa.NewGlimpse(get, irsa.GlimpseII)
	if err != nil {
		t.Fatal(err)
	}
	return glimpse
}

// Sgr A* falls in the tile at the galactic centre and in none beside it.
// The tile brings its mosaic in each band.
func TestFindNamesTheTilesMosaicInEachBand(t *testing.T) {
	glimpse := glimpseOf(t, "glimpse-ii.html")
	products, err := glimpse.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != bands {
		t.Fatalf("%d products", len(products))
	}
	for _, product := range products {
		if !strings.HasPrefix(product.Name(), "GLM_00000+0000_mosaic_I") ||
			product.URL() != irsa.GlimpseII+product.Name() ||
			product.ID() != "GLM_00000+0000" ||
			product.Archive() != glimpse.Name() ||
			!strings.HasPrefix(product.About(), "GLIMPSE mosaic, IRAC I") {
			t.Errorf("%s %s %s %q", product.Name(), product.URL(),
				product.ID(), product.About())
		}
	}
}

// Links out of the folder are not its mosaics, nor are headers.
func TestFindKeepsOnlyTheFoldersMosaics(t *testing.T) {
	products, err := glimpseOf(t, "leading-out.html").
		Find(replay.Asked(t))
	if err != nil || len(products) != len("1") ||
		products[first].Name() != "GLM_00000+0000_mosaic_I4.fits" {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-tests: a tile whose size will not read is an error, and so is a
// folder that will not list.
func TestFindReportsWhatItCannotRead(t *testing.T) {
	_, err := glimpseOf(t, "crooked-size.html").Find(replay.Asked(t))
	if err == nil || !strings.Contains(err.Error(), "3.1.2") {
		t.Errorf("%v", err)
	}
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(irsa.GlimpseII).Return(nil, int64(unsized), down)
	glimpse, err := irsa.NewGlimpse(get, irsa.GlimpseII)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := glimpse.Find(replay.Asked(t)); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}

// Anti-tests: NewGlimpse refuses no web, and a folder that is not one.
func TestNewGlimpseRefusesWhatCannotList(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	folder := strings.TrimSuffix(irsa.GlimpseII, "/")
	if _, err := irsa.NewGlimpse(get, folder); !errors.Is(err,
		celestialdata.Refused()) {
		t.Errorf("a folder with no slash: %v", err)
	}
	if _, err := irsa.NewGlimpse(nil, irsa.GlimpseII); !errors.Is(err,
		celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
}

// The catalogue's file is the table's rows in the circle. They are asked
// of the archive's TAP service in CSV. The name says where.
func TestACatalogueNamesItsRowsInTheCircle(t *testing.T) {
	catalogue, err := irsa.NewCatalogue("galcen_psc", "*")
	if err != nil {
		t.Fatal(err)
	}
	products, err := catalogue.Find(replay.Asked(t))
	if err != nil || len(products) != len("1") {
		t.Fatalf("%v, %v", products, err)
	}
	product := products[first]
	adql := "SELECT * FROM galcen_psc WHERE " +
		tap.Within(replay.Asked(t), "ra", "dec")
	address := tap.URL("https://irsa.ipac.caltech.edu/TAP/sync", adql,
		tap.CSV)
	if product.URL() != address || catalogue.Name() != "galcen_psc" ||
		product.Archive() != catalogue.Name() ||
		product.Name() != "galcen_psc_266.4168371_-29.0078106_r0.1.csv" ||
		product.About() != "the rows of galcen_psc within 0.1 degrees" {
		t.Errorf("%s %s %q", product.URL(), product.Name(),
			product.About())
	}
}

// Anti-tests: a catalogue needs a table and columns, and a table whose
// name cannot be a file's has no file.
func TestACatalogueRefusesWhatCannotBeAsked(t *testing.T) {
	for _, each := range []struct{ table, columns string }{
		{"", "*"}, {"galcen_psc", " "}} {
		_, err := irsa.NewCatalogue(each.table, each.columns)
		if !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%q: %v", each, err)
		}
	}
	slashed, err := irsa.NewCatalogue("galcen/psc", "*")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := slashed.Find(replay.Asked(t)); !errors.Is(err,
		celestialdata.Refused()) {
		t.Errorf("a slashed table: %v", err)
	}
}

// The files found come whole: each is of the size the adapter gave it.
func TestFoundFilesComeWhole(t *testing.T) {
	products, err := glimpseOf(t, "glimpse-ii.html").Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := irsa.NewCatalogue("galcen_psc", "*")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := catalogue.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, product := range []*celestialdata.Product{products[first], rows[first]} {
		if err := replay.Fetched(t, product); err != nil {
			t.Errorf("%s: %v", product.Name(), err)
		}
	}
}

// Each part of a tile's name places it. Sgr A* reaches the tile named at
// 359.94 in hundredths, and the one at 0.30 by half its 0.6 width. It does
// not reach the tile three degrees north, nor the short one half a degree
// north.
func TestATilesNamePlacesIt(t *testing.T) {
	products, err := glimpseOf(t, "placed-tiles.html").Find(replay.Asked(t))
	var names []string
	for _, product := range products {
		names = append(names, product.Name())
	}
	want := []string{"GLM_35994+0000_mosaic_I4.fits",
		"GLM_00030+0000_mosaic_I4.fits"}
	if err != nil || !slices.Equal(names, want) {
		t.Errorf("%q, %v", names, err)
	}
}
