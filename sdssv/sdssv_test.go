package sdssv_test

import (
	"errors"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/sdssv"
)

// The searches the recorded answers asked, each defined by the query
// it was recorded from on 2026-11-06.
const (
	most      = 3         // defined: the most spectra each recorded search asked for
	threeRA   = 41.728048 // defined: the centre of the search with three spectra
	threeDec  = 0         // defined: its declination
	threeWide = 0.5       // defined: its radius, degrees
	adapter   = "sdssv"   // defined: the archive's name
	none      = 0         // defined: a search that asks for none
	least     = 1         // defined: the fewest spectra a search may ask for
	runs      = "https://dr19.sdss.org/sas/dr19/spectro/boss/redux/v6_1_3/spectra/full/015015/"
)

// three is the circle whose recorded answer holds three spectra, or the
// test stops.
func three(test *testing.T) *celestialdata.Near {
	test.Helper()
	centre, err := celestialdata.NewPosition(threeRA, threeDec)
	if err != nil {
		test.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, threeWide)
	if err != nil {
		test.Fatal(err)
	}
	return near
}

// answering is the archive asked through a web that answers a circle's
// search with a recorded or crafted file, or the test stops.
func answering(test *testing.T, near *celestialdata.Near,
	file string) *sdssv.Archive {
	test.Helper()
	asked := sdssv.Asking(near, most)
	get := replay.Web(test, map[string]string{asked: file})
	archive, err := sdssv.New(get, most)
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// the test: the archive is named, asked by name only, and refuses a
// search for fewer than one spectrum.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := sdssv.New(get, most)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	one, err := sdssv.New(get, least)
	if one == nil || err != nil {
		test.Errorf("most one: %v", err)
	}
	nowhere, err := sdssv.New(nil, most)
	if nowhere != nil || err == nil {
		test.Errorf("no web: %v", err)
	}
	refused, err := sdssv.New(get, none)
	if refused != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "at least one spectrum") {
		test.Errorf("most none: %v", err)
	}
}

// spectrum is a recorded spectrum's file name, its MJD and its median
// signal to noise, as the recorded answer gives them.
type spectrum struct{ name, mjd, median string }

// the test: a search SkyServer answers with three spectra finds the
// three files, the clearest first, each where the archive keeps it; a
// search it answers with none finds none.
func TestASearchFindsEachSpectrumWhereItIsKept(test *testing.T) {
	near := three(test)
	products, err := answering(test, near, "spall-three.csv").Find(near)
	if err != nil {
		test.Fatal(err)
	}
	recorded := []spectrum{
		{"spec-015015-59169-4401931560.fits", "59169", "51.56928"},
		{"spec-015015-59192-4401931560.fits", "59192", "50.80331"},
		{"spec-015015-59222-4401931560.fits", "59222", "47.45237"}}
	if len(products) != len(recorded) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range recorded {
		product := products[at]
		id := strings.TrimSuffix(strings.TrimPrefix(want.name, "spec-"),
			".fits")
		if product.Name() != want.name || product.Archive() != adapter ||
			product.ID() != id || product.URL() != runs+want.mjd+"/"+want.name ||
			product.About() != "a BOSS spectrum, star, median S/N "+want.median {
			test.Errorf("%s %s %s %q", product.Name(), product.ID(),
				product.URL(), product.About())
		}
	}
	sgra := replay.Asked(test)
	found, err := answering(test, sgra, "spall-sgra.csv").Find(sgra)
	if found != nil || err != nil {
		test.Errorf("round Sgr A*: %v, %v", found, err)
	}
}

// the anti-test: an answer the archive cannot read is refused, saying
// why: one that is no table, one that lacks a column the products need,
// one whose quoting is broken, and file names not of a spectrum's form.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	near := three(test)
	broken := map[string]string{
		"crafted-no-table.csv":       "the answer is not one table",
		"crafted-table-alone.csv":    "the answer is not one table",
		"crafted-empty.csv":          "the answer is not one table",
		"crafted-table-line-two.csv": "the answer is not one table",
		"crafted-other-table.csv":    "the answer is not one table",
		"crafted-short-row.csv":      "a row of 2 fields under 4 columns",
		"crafted-no-sn.csv":          "the answer has no sn_median_all",
		"crafted-bad-quote.csv":      "parse error",
		"crafted-two-parts.csv":      `"spec-015015-59169.fits" is not`,
		"crafted-not-fits.csv":       `"spec-015015-59169-4401931560.txt" is not`,
		"crafted-not-spec.csv":       `"boss-015015-59169-4401931560.fits" is not`,
		"crafted-empty-part.csv":     `"spec--59169-4401931560.fits" is not`}
	for file, said := range broken {
		found, err := answering(test, near, file).Find(near)
		if found != nil || err == nil || !strings.Contains(err.Error(), said) {
			test.Errorf("%s: %v, %v", file, found, err)
		}
	}
}

// the anti-test: a search the web cannot answer is refused, by the
// archive's name.
func TestASearchTheWebCannotAnswerIsRefused(test *testing.T) {
	near := three(test)
	get := mocks.NewMockWeb(gomock.NewController(test))
	unreachable := errors.New("no route to SkyServer")
	get.EXPECT().Get(sdssv.Asking(near, most)).Return(nil, int64(none),
		unreachable)
	archive, err := sdssv.New(get, most)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(near)
	if found != nil || !errors.Is(err, unreachable) ||
		!strings.HasPrefix(err.Error(), adapter) {
		test.Errorf("%v, %v", found, err)
	}
}

// recordedAt is the address the three spectra's answer was recorded
// from on 2026-11-06, as the testdata readme says: a circle of 30
// arcminutes round RA 41.728048, Dec 0, at most 3.
const recordedAt = "https://skyserver.sdss.org/dr19/SkyServerWS/SearchTools/" +
	"SqlSearch?cmd=SELECT+TOP+3+spec_file%2C+run2d%2C+class%2C+" +
	"sn_median_all+FROM+spAll+WHERE+dbo.fDistanceArcMinEq%2841.728048%2C+" +
	"0%2C+racat%2C+deccat%29+%3C%3D+30+ORDER+BY+sn_median_all+DESC&format=csv"

// the test: a search asks SkyServer for its circle as the recorded
// answer was asked: the centre in degrees, the radius in arcminutes,
// each number in its fewest digits.
func TestASearchAsksAsTheAnswerWasRecorded(test *testing.T) {
	if asked := sdssv.Asking(three(test), most); asked != recordedAt {
		test.Errorf("asks %s", asked)
	}
}
