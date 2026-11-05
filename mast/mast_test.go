package mast_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mast"
	"github.com/janearc/celestial-data/mocks"
	"go.uber.org/mock/gomock"
)

// folder is where MAST keeps the survey's files.
const folder = "https://archive.stsci.edu/pub/hlsp/hpsgc/"

// The counts and places the tests work with.
const (
	files   = 5   // defined: the FITS files the recorded listing holds
	first   = 0   // defined: the first product
	unsized = 0   // defined: the size of a file the archive does not size
	north   = 60  // defined: a declination far from the Galactic Centre
	east    = 0   // defined: a right ascension far from it too
	wide    = 0.1 // defined: the radius of the circle far away
)

// surveyOf is the survey asked through get, over its own field.
func surveyOf(t *testing.T, get *mocks.MockWeb) *mast.Survey {
	t.Helper()
	field, err := mast.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	survey, err := mast.New(get, field)
	if err != nil {
		t.Fatal(err)
	}
	return survey
}

// Sgr A* is in the survey's field, so its every FITS file is found.
func TestFindNamesTheSurveysFiles(t *testing.T) {
	get := replay.Web(t, map[string]string{folder: "hpsgc.html"})
	survey := surveyOf(t, get)
	products, err := survey.Find(replay.Asked(t))
	if err != nil || len(products) != files {
		t.Fatalf("%d products, %v", len(products), err)
	}
	for _, product := range products {
		if !strings.HasPrefix(product.Name(), "hlsp_hpsgc_") ||
			product.URL() != folder+product.Name() ||
			product.Archive() != survey.Name() ||
			!strings.Contains(product.About(), "Paschen-alpha") {
			t.Errorf("%s %s %q", product.Name(), product.URL(),
				product.About())
		}
	}
}

// Circles far from the field find nothing and ask nothing.
func TestFindAsksNothingFarFromTheField(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	survey := surveyOf(t, get)
	centre, err := celestialdata.NewPosition(east, north)
	if err != nil {
		t.Fatal(err)
	}
	far, err := celestialdata.NewNear(centre, wide)
	if err != nil {
		t.Fatal(err)
	}
	if products, err := survey.Find(far); products != nil || err != nil {
		t.Errorf("%v, %v", products, err)
	}
}

// Links out of the folder are not its files.
func TestFindKeepsOnlyTheFoldersFiles(t *testing.T) {
	get := replay.Web(t, map[string]string{folder: "leading-out.html"})
	products, err := surveyOf(t, get).Find(replay.Asked(t))
	if err != nil || len(products) != len("1") || products[first].Name() !=
		"hlsp_hpsgc_hst_nicmos-nic3_gc_palpha_v1_img.fits" {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-tests: a folder that will not list is an error, and New refuses
// no web and no field.
func TestWhatCannotBeAskedIsAnError(t *testing.T) {
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(folder).Return(nil, int64(unsized), down)
	if _, err := surveyOf(t, get).Find(replay.Asked(t)); !errors.Is(err,
		down) {
		t.Errorf("%v", err)
	}
	field, err := mast.Footprint()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mast.New(nil, field); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
	if _, err := mast.New(get, nil); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no field: %v", err)
	}
}

// The files found come whole: each is of the size the adapter gave it.
func TestFoundFilesComeWhole(t *testing.T) {
	get := replay.Web(t, map[string]string{folder: "hpsgc.html"})
	products, err := surveyOf(t, get).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, products[first]); err != nil {
		t.Errorf("%v", err)
	}
}
