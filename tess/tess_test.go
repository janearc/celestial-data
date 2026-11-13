package tess_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tess"
)

// The recorded search: firang's star, Gaia DR3 4081136684118965760, in a
// circle of 0.003 degrees, asked of MAST on 2026-11-06. Each defined by
// what MAST answered.
const (
	starRA      = 287.50086 // defined: the star's right ascension, degrees
	starDec     = -22.23481 // defined: its declination
	starWide    = 0.003     // defined: the circle's radius, degrees
	adapter     = "tess"    // defined: the archive's name
	lightCurve  = "hlsp_qlp_tess_ffi_s0092-0000000188067735_tess_v01_llc.fits"
	observation = "296214224" // defined: the light curve's CAOM obsid
	curveBytes  = 685440      // defined: its size, as MAST lists it
	curveURL    = "https://mast.stsci.edu/api/v0.1/Download/file?uri=" +
		"mast%3AHLSP%2Fqlp%2Fs0092%2F0000%2F0001%2F8806%2F7735%2F" + lightCurve
	curveAbout = "a TESS light curve of 188067735, sector 92, 200 s " +
		"cadence, from QLP"
	lone       = 1  // defined: one light curve
	first      = 0  // defined: its place
	mostAsks   = 10 // defined: the asks the adapter allows a question
	noLength   = 0  // defined: a failed answer's length
	twoProduct = 2  // defined: the preview answer's products
)

// star is the recorded circle, or the test stops.
func star(test *testing.T) *celestialdata.Near {
	test.Helper()
	centre, err := celestialdata.NewPosition(starRA, starDec)
	if err != nil {
		test.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, starWide)
	if err != nil {
		test.Fatal(err)
	}
	return near
}

// asked is the address a recorded search asked, or the test stops.
func asked(test *testing.T, file string) string {
	test.Helper()
	recorded, err := os.ReadFile("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	return strings.TrimSpace(string(recorded))
}

// answering is the archive asked through a web that answers the cone
// and the products with the files given, or the test stops.
func answering(test *testing.T, coneFile, productsFile string) *tess.Archive {
	test.Helper()
	replies := map[string]string{asked(test, "asked-cone.txt"): coneFile}
	if productsFile != "" {
		replies[asked(test, "asked-products.txt")] = productsFile
	}
	archive, err := tess.New(replay.Web(test, replies))
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// the test: the archive is named, asked by name only, and refuses no
// web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := tess.New(get)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	if nowhere, err := tess.New(nil); nowhere != nil || err == nil {
		test.Errorf("no web: %v", err)
	}
}

// the test: a search MAST answered with eleven observations finds the
// one TESS light curve among them, QLP's of sector 92, fetched from
// MAST's download by its data URI; the full-frame image is not found.
func TestASearchFindsTheLightCurve(test *testing.T) {
	archive := answering(test, "cone.json", "products.json")
	products, err := archive.Find(star(test))
	if err != nil || len(products) != lone {
		test.Fatalf("%d products, %v", len(products), err)
	}
	curve := products[first]
	if curve.Archive() != adapter || curve.ID() != observation ||
		curve.Name() != lightCurve || curve.URL() != curveURL ||
		curve.About() != curveAbout {
		test.Errorf("%s %s %s %s %q", curve.Archive(), curve.ID(),
			curve.Name(), curve.URL(), curve.About())
	}
}

// the test: a product that is not a science file is left out.
func TestOnlyScienceFilesAreFound(test *testing.T) {
	archive := answering(test, "cone.json", "products-preview.json")
	products, err := archive.Find(star(test))
	if err != nil || len(products) != lone ||
		products[first].Name() != lightCurve {
		test.Errorf("%d products, %v", len(products), err)
	}
}

// the test: a question MAST is still working on is asked again, and
// read when MAST is done.
func TestAQuestionStillExecutingIsAskedAgain(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	cone := asked(test, "asked-cone.txt")
	working, err := os.Open("testdata/executing.json")
	if err != nil {
		test.Fatal(err)
	}
	done, err := os.Open("testdata/cone.json")
	if err != nil {
		test.Fatal(err)
	}
	listed, err := os.Open("testdata/products.json")
	if err != nil {
		test.Fatal(err)
	}
	none := int64(noLength)
	listedAt := asked(test, "asked-products.txt")
	stillWorking := get.EXPECT().Get(cone).Return(working, none, nil)
	finished := get.EXPECT().Get(cone).Return(done, none, nil)
	listing := get.EXPECT().Get(listedAt).Return(listed, none, nil)
	gomock.InOrder(stillWorking, finished, listing)
	archive, err := tess.New(get)
	if err != nil {
		test.Fatal(err)
	}
	products, err := archive.Find(star(test))
	if err != nil || len(products) != lone {
		test.Errorf("%d products, %v", len(products), err)
	}
}

// the anti-test: a question still executing after the asks allowed is
// refused, saying so.
func TestAQuestionNeverDoneIsRefused(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	cone := asked(test, "asked-cone.txt")
	for range mostAsks {
		working, err := os.Open("testdata/executing.json")
		if err != nil {
			test.Fatal(err)
		}
		get.EXPECT().Get(cone).Return(working, int64(noLength), nil)
	}
	archive, err := tess.New(get)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(star(test))
	if found != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "still executing after 10 asks") {
		test.Errorf("%v, %v", found, err)
	}
}

// refused is an answer the archive cannot read, and its refusal's words.
type refused struct{ cone, products, said string }

// the anti-test: an answer the archive cannot read is refused, by its
// name, saying why: one that is not MAST's, one MAST says failed, rows
// that are not rows, and a product whose name is not a plain file's.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	for _, each := range []refused{
		{"not-json.json", "", "not MAST's"},
		{"error.json", "", "Mast.Caom.Cone says ERROR: the service broke"},
		{"not-rows.json", "", "Mast.Caom.Cone's rows"},
		{"cone.json", "error.json", "Mast.Caom.Products says ERROR"},
		{"cone.json", "products-not-plain.json", "plain"}} {
		archive := answering(test, each.cone, each.products)
		found, err := archive.Find(star(test))
		if found != nil || err == nil || !strings.Contains(err.Error(),
			each.said) || !named(err) {
			test.Errorf("%s, %s: %v, %v", each.cone, each.products, found, err)
		}
	}
}

// the anti-test: a search the web cannot answer is refused, by the
// archive's name, whether it is the cone's question or the products'.
func TestASearchTheWebCannotAnswerIsRefused(test *testing.T) {
	unreachable := errors.New("no route to MAST")
	cone := asked(test, "asked-cone.txt")
	for _, failing := range []string{cone, asked(test, "asked-products.txt")} {
		get := mocks.NewMockWeb(gomock.NewController(test))
		if failing != cone {
			recorded, err := os.Open("testdata/cone.json")
			if err != nil {
				test.Fatal(err)
			}
			get.EXPECT().Get(cone).Return(recorded, int64(noLength), nil)
		}
		get.EXPECT().Get(failing).Return(nil, int64(noLength), unreachable)
		archive, err := tess.New(get)
		if err != nil {
			test.Fatal(err)
		}
		found, err := archive.Find(star(test))
		if found != nil || !errors.Is(err, unreachable) ||
			!named(err) {
			test.Errorf("%v, %v", found, err)
		}
	}
}

// named is whether an error names the archive, as a failure does
// ("tess: ...") or a refusal ("refused: tess ...").
func named(err error) bool {
	said := err.Error()
	return strings.HasPrefix(said, adapter+":") ||
		strings.HasPrefix(said, "refused: "+adapter+" ")
}

// the anti-test: an answer the web breaks off while it is read is
// refused, by the archive's name, wrapping why.
func TestAnAnswerBrokenOffIsRefused(test *testing.T) {
	broken := errors.New("the connection dropped")
	get := mocks.NewMockWeb(gomock.NewController(test))
	body := io.NopCloser(iotest.ErrReader(broken))
	get.EXPECT().Get(asked(test, "asked-cone.txt")).Return(body,
		int64(noLength), nil)
	archive, err := tess.New(get)
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(star(test))
	if found != nil || !errors.Is(err, broken) || !named(err) {
		test.Errorf("%v, %v", found, err)
	}
}
