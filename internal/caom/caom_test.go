package caom_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/caom"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/web"
)

// The recorded search: firang's star, Gaia DR3 4081136684118965760, in a
// circle of 0.003 degrees, asked of MAST on 2026-11-06 for the tess
// adapter. Each defined by what MAST answered.
const (
	starRA       = 287.50086 // defined: the star's right ascension
	starDec      = -22.23481 // defined: its declination
	starWide     = 0.003     // defined: the circle's radius, degrees
	asker        = "tess"    // defined: the adapter the answers were for
	observations = 11        // defined: the cone's rows
	curveObsID   = 296214224 // defined: the light curve's obsid
	curveFiles   = 1         // defined: that observation's files
	curveFile    = "hlsp_qlp_tess_ffi_s0092-0000000188067735_tess_v01_llc.fits"
	curveSize    = 685440 // defined: its size, as MAST lists it
	sector       = 92     // defined: the light curve's sector
	mostAsks     = 10     // defined: the asks numbers.yaml allows
	noLength     = 0      // defined: a failed answer's length
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

// asked is the address a recorded question asked, or the test stops.
func asked(test *testing.T, file string) string {
	test.Helper()
	recorded, err := os.ReadFile("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	return strings.TrimSpace(string(recorded))
}

// opened is a recorded answer, or the test stops.
func opened(test *testing.T, file string) *os.File {
	test.Helper()
	answer, err := os.Open("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	return answer
}

// through is an asker for the tess adapter, asking through get.
func through(test *testing.T, get web.Web) *caom.Asker {
	test.Helper()
	base, err := celestialdata.NewAdapter(asker, get)
	if err != nil {
		test.Fatal(err)
	}
	return caom.New(base)
}

// answering is an asker whose web answers each recorded question with
// the file given.
func answering(test *testing.T, cone, products string) *caom.Asker {
	test.Helper()
	replies := map[string]string{}
	if cone != "" {
		replies[asked(test, "asked-cone.txt")] = cone
	}
	if products != "" {
		replies[asked(test, "asked-products.txt")] = products
	}
	return through(test, replay.Web(test, replies))
}

// the test: a cone search reads each observation MAST gave, its fields
// by MAST's names.
func TestAConeReadsEachObservation(test *testing.T) {
	seen, err := answering(test, "cone.json", "").Cone(star(test))
	if err != nil || len(seen) != observations {
		test.Fatalf("%d observations, %v", len(seen), err)
	}
	var curve *caom.Observation
	for at := range seen {
		if seen[at].ObsID == curveObsID {
			curve = &seen[at]
		}
	}
	if curve == nil || curve.Project != "TESS" ||
		curve.Kind != "timeseries" || curve.Sequence != sector ||
		curve.Provenance != "QLP" || curve.Target != "188067735" {
		test.Errorf("%+v", curve)
	}
}

// the test: an observation's files are read with their names, kinds,
// data URIs and sizes, and each is fetched from MAST's download.
func TestFilesAreReadAndFetchedByURI(test *testing.T) {
	files, err := answering(test, "", "products.json").Files(curveObsID)
	if err != nil || len(files) != curveFiles {
		test.Fatalf("%d files, %v", len(files), err)
	}
	var curve *caom.File
	for at := range files {
		if files[at].Name == curveFile {
			curve = &files[at]
		}
	}
	want := "https://mast.stsci.edu/api/v0.1/Download/file?uri=" +
		"mast%3AHLSP%2Fqlp%2Fs0092%2F0000%2F0001%2F8806%2F7735%2F" +
		curveFile
	if curve == nil || curve.Size != curveSize ||
		curve.Kind != "SCIENCE" || caom.Download(curve.URI) != want {
		test.Errorf("%+v", curve)
	}
}

// the test: a question MAST is still working on is asked again, and
// read when MAST is done.
func TestAQuestionStillExecutingIsAskedAgain(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	cone, none := asked(test, "asked-cone.txt"), int64(noLength)
	gomock.InOrder(
		get.EXPECT().Get(cone).Return(opened(test, "executing.json"),
			none, nil),
		get.EXPECT().Get(cone).Return(opened(test, "cone.json"), none,
			nil))
	seen, err := through(test, get).Cone(star(test))
	if err != nil || len(seen) != observations {
		test.Errorf("%d observations, %v", len(seen), err)
	}
}

// the anti-test: a question still executing after the asks allowed is
// refused, saying so, and not asked once more.
func TestAQuestionNeverDoneIsRefused(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	cone := asked(test, "asked-cone.txt")
	for range mostAsks {
		get.EXPECT().Get(cone).Return(opened(test, "executing.json"),
			int64(noLength), nil)
	}
	seen, err := through(test, get).Cone(star(test))
	if seen != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "still executing after 10 asks") {
		test.Errorf("%v, %v", seen, err)
	}
}

// refusing is an answer the asker cannot read, and its refusal's words.
type refusing struct{ file, said string }

// the anti-test: an answer the asker cannot read is refused in the
// adapter's name, saying why: one that is not MAST's, one MAST says
// failed, and rows that are not rows.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	for _, each := range []refusing{
		{"not-json.json", "not MAST's"},
		{"error.json", "Mast.Caom.Cone says ERROR: the service broke"},
		{"not-rows.json", "Mast.Caom.Cone's rows"}} {
		seen, err := answering(test, each.file, "").Cone(star(test))
		if seen != nil || err == nil || !named(err) ||
			!strings.Contains(err.Error(), each.said) {
			test.Errorf("%s: %v, %v", each.file, seen, err)
		}
	}
	files, err := answering(test, "", "error.json").Files(curveObsID)
	if files != nil || err == nil || !named(err) ||
		!strings.Contains(err.Error(), "Mast.Caom.Products says ERROR") {
		test.Errorf("products: %v, %v", files, err)
	}
}

// the anti-test: a question the web cannot answer, or whose answer
// breaks off, is refused in the adapter's name, wrapping why.
func TestAnAnswerNotHadIsRefused(test *testing.T) {
	cone := asked(test, "asked-cone.txt")
	for _, cause := range []error{errors.New("no route to MAST"),
		errors.New("the connection dropped")} {
		get := mocks.NewMockWeb(gomock.NewController(test))
		if cause.Error() == "no route to MAST" {
			get.EXPECT().Get(cone).Return(nil, int64(noLength), cause)
		} else {
			body := io.NopCloser(iotest.ErrReader(cause))
			get.EXPECT().Get(cone).Return(body, int64(noLength), nil)
		}
		seen, err := through(test, get).Cone(star(test))
		if seen != nil || !errors.Is(err, cause) || !named(err) {
			test.Errorf("%v, %v", seen, err)
		}
	}
}

// named is whether an error names the adapter, as a failure does
// ("tess: ...") or a refusal ("refused: tess ...").
func named(err error) bool {
	said := err.Error()
	return strings.HasPrefix(said, asker+":") ||
		strings.HasPrefix(said, "refused: "+asker+" ")
}
