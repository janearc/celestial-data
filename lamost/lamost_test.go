package lamost_test

import (
	"errors"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/lamost"
	"github.com/janearc/celestial-data/mocks"
)

// The searches the recorded answers asked, each defined by the query
// it was recorded from on 2026-11-06.
const (
	most      = 3        // defined: the most of each resolution a recorded search asked for
	least     = 1        // defined: the fewest a search may ask for
	none      = 0        // defined: a search that asks for none
	threeRA   = 180      // defined: the centre of the search with three of each
	threeDec  = 30       // defined: its declination
	threeWide = 0.5      // defined: its radius, degrees
	adapter   = "lamost" // defined: the archive's name
	low       = "https://www.lamost.org/dr10/v2.0/spectrum/fits/"
	medium    = "https://www.lamost.org/dr10/v2.0/medspectrum/fits/"
)

// three is the circle whose recorded answers hold three spectra of each
// resolution, or the test stops.
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

// asked are the two addresses a recorded search asked, low resolution
// first, as recorded beside the answers, or the test stops.
func asked(test *testing.T, file string) (string, string) {
	test.Helper()
	recorded, err := os.ReadFile("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	trimmed := strings.TrimSpace(string(recorded))
	lowAsked, mediumAsked, found := strings.Cut(trimmed, "\n")
	if !found {
		test.Fatalf("%s holds no two addresses", file)
	}
	return lowAsked, mediumAsked
}

// answering is the archive asked through a web that answers the
// recorded addresses with the files given, or the test stops.
func answering(test *testing.T, addresses, lowFile,
	mediumFile string) *lamost.Archive {
	test.Helper()
	lowAsked, mediumAsked := asked(test, addresses)
	replies := map[string]string{lowAsked: lowFile}
	if mediumFile != "" {
		replies[mediumAsked] = mediumFile
	}
	archive, err := lamost.New(replay.Web(test, replies), most)
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// the test: the archive is named, asked by name only, and asks for at
// least one spectrum of each resolution; it refuses none, and no web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := lamost.New(get, least)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	refused, err := lamost.New(get, none)
	if refused != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "at least one spectrum") {
		test.Errorf("most none: %v", err)
	}
	if nowhere, err := lamost.New(nil, most); nowhere != nil || err == nil {
		test.Errorf("no web: %v", err)
	}
}

// spectrum is a recorded spectrum as a product should give it.
type spectrum struct{ obsid, name, about string }

// the test: a search the service answers with three spectra of each
// resolution finds the six files, low resolution first and each the
// clearest first, each fetched by its obsid under the release's own
// name; a search south of LAMOST's sky finds none.
func TestASearchFindsEachSpectrumByItsObsid(test *testing.T) {
	near := three(test)
	archive := answering(test, "asked-three.txt", "low-three.xml",
		"medium-three.xml")
	products, err := archive.Find(near)
	if err != nil {
		test.Fatal(err)
	}
	recorded := []spectrum{
		{"332714134", "spec-57094-HD121027N290405V01_sp14-134.fits.gz",
			"a LAMOST low-resolution spectrum, STAR G6, S/N 441.1 in r"},
		{"332714122", "spec-57094-HD121027N290405V01_sp14-122.fits.gz",
			"a LAMOST low-resolution spectrum, STAR K5, S/N 337.2 in r"},
		{"332716013", "spec-57094-HD121027N290405V01_sp16-013.fits.gz",
			"a LAMOST low-resolution spectrum, STAR F0, S/N 237.4 in r"},
		{"788910113", "med-58862-NT121316N301658M01_sp10-113.fits.gz",
			"a LAMOST medium-resolution spectrum, both arms, S/N 183.7 in the red"},
		{"788910128", "med-58862-NT121316N301658M01_sp10-128.fits.gz",
			"a LAMOST medium-resolution spectrum, both arms, S/N 47.1 in the red"},
		{"788910110", "med-58862-NT121316N301658M01_sp10-110.fits.gz",
			"a LAMOST medium-resolution spectrum, both arms, S/N 36.1 in the red"}}
	if len(products) != len(recorded) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range recorded {
		product, files := products[at], low
		if strings.HasPrefix(want.name, "med-") {
			files = medium
		}
		if product.ID() != want.obsid || product.Name() != want.name ||
			product.URL() != files+want.obsid || product.About() != want.about ||
			product.Archive() != adapter {
			test.Errorf("%s %s %s %q", product.ID(), product.Name(),
				product.URL(), product.About())
		}
	}
	south := answering(test, "asked-sgra.txt", "low-sgra.xml",
		"medium-sgra.xml")
	found, err := south.Find(replay.Asked(test))
	if found != nil || err != nil {
		test.Errorf("round Sgr A*: %v, %v", found, err)
	}
}

// the anti-test: an answer the archive cannot read is refused, saying
// which column it lacks, and so is a spectrum with no file, low or
// medium; once the low answer fails, the medium one is not asked for.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	near := three(test)
	lowBroken := map[string]string{
		"crafted-low-no-obsid.xml":    "obsid",
		"crafted-low-no-class.xml":    "class",
		"crafted-low-no-subclass.xml": "subclass",
		"crafted-low-no-snrr.xml":     "snrr",
		"crafted-low-no-file.xml":     "file",
		"crafted-low-empty-file.xml":  `"."`}
	for file, said := range lowBroken {
		archive := answering(test, "asked-three.txt", file, "")
		found, err := archive.Find(near)
		if found != nil || err == nil || !strings.Contains(err.Error(), said) ||
			!strings.HasPrefix(err.Error(), adapter) {
			test.Errorf("%s: %v, %v", file, found, err)
		}
	}
	empty := answering(test, "asked-three.txt", "low-three.xml",
		"crafted-medium-empty-file.xml")
	found, err := empty.Find(near)
	if found != nil || err == nil || !strings.Contains(err.Error(), `"."`) {
		test.Errorf("medium with an empty file: %v, %v", found, err)
	}
	archive := answering(test, "asked-three.txt", "low-three.xml",
		"crafted-medium-no-snr.xml")
	found, err = archive.Find(near)
	if found != nil || err == nil || !strings.Contains(err.Error(), "snr") {
		test.Errorf("medium with no snr: %v, %v", found, err)
	}
}

// the anti-test: a search the web cannot answer is refused, by the
// archive's name, whichever resolution's question it is.
func TestASearchTheWebCannotAnswerIsRefused(test *testing.T) {
	near := three(test)
	lowAsked, mediumAsked := asked(test, "asked-three.txt")
	unreachable := errors.New("no route to LAMOST")
	for _, failing := range []string{lowAsked, mediumAsked} {
		get := mocks.NewMockWeb(gomock.NewController(test))
		if failing == mediumAsked {
			recorded, err := os.Open("testdata/low-three.xml")
			if err != nil {
				test.Fatal(err)
			}
			get.EXPECT().Get(lowAsked).Return(recorded, int64(none), nil)
		}
		get.EXPECT().Get(failing).Return(nil, int64(none), unreachable)
		archive, err := lamost.New(get, most)
		if err != nil {
			test.Fatal(err)
		}
		found, err := archive.Find(near)
		if found != nil || !errors.Is(err, unreachable) ||
			!strings.HasPrefix(err.Error(), adapter) {
			test.Errorf("%v, %v", found, err)
		}
	}
}
