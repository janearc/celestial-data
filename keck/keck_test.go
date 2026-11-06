package keck_test

import (
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/keck"
	"github.com/janearc/celestial-data/mocks"
)

// The searches the recorded answers asked, each defined by the query
// it was recorded from on 2026-11-06.
const (
	most        = 2         // defined: the most frames a recorded search asked for
	least       = 1         // defined: the fewest a search may ask for
	none        = 0         // defined: a search that asks for none
	twoRA       = 148.49225 // defined: the centre of the search with two frames
	twoDec      = 57.68342  // defined: its declination
	twoWide     = 0.01      // defined: its radius, degrees
	adapter     = "keck"    // defined: the archive's name
	koa         = "https://koa.ipac.caltech.edu/cgi-bin/"
	spring      = "HI.20070426.25454.fits" // defined: the first recorded frame
	autumn      = "HI.20071123.53347.fits" // defined: the second
	springHand  = "/koadata2/HIRES/20070426/lev0/HI.20070426.25454.fits"
	autumnHand  = "/koadata3/HIRES/20071123/lev0/HI.20071123.53347.fits"
	springAbout = "a HIRES frame of 85472, 2007-04-26 07:04:14.420000, " +
		"76 s through B5, PI Winn"
	autumnAbout = "a HIRES frame of 85472, 2007-11-23 14:49:07.640000, " +
		"0 s through B5, PI D.Fischer"
	extracted = ", extracted by MAKEE"
	once      = 1 // defined: a replacement made the first time only
)

// two is the circle whose recorded answer holds two frames, or the
// test stops.
func two(test *testing.T) *celestialdata.Near {
	test.Helper()
	centre, err := celestialdata.NewPosition(twoRA, twoDec)
	if err != nil {
		test.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, twoWide)
	if err != nil {
		test.Fatal(err)
	}
	return near
}

// asked is the address a recorded search asked, as recorded beside the
// answer, or the test stops.
func asked(test *testing.T, file string) string {
	test.Helper()
	recorded, err := os.ReadFile("testdata/" + file)
	if err != nil {
		test.Fatal(err)
	}
	return strings.TrimSpace(string(recorded))
}

// listedAt is where KOA lists a frame's level-1 files.
func listedAt(koaid, filehand string) string {
	return koa + "KoaAPI/nph-getL1list?" + url.Values{
		"instrument": {"HIRES"}, "koaid": {koaid},
		"filehand": {filehand}}.Encode()
}

// rawAt is where KOA serves a frame's raw file.
func rawAt(filehand string) string {
	return koa + "getKOA/nph-getKOA?" + url.Values{
		"filehand": {filehand}}.Encode()
}

// reducedAt is where KOA serves one of a frame's level-1 files.
func reducedAt(koaid, kept string) string {
	return koa + "KoaAPI/nph-dnloadL1data?" + url.Values{
		"instrument": {"HIRES"}, "koaid": {koaid}, "filehand": {kept}}.Encode()
}

// answering is the archive asked through a web that answers the two
// frames' search with a frames file, and each frame's level-1 list
// with the files given, or the test stops.
func answering(test *testing.T, frames, springList,
	autumnList string) *keck.Archive {
	test.Helper()
	replies := map[string]string{asked(test, "asked-two.txt"): frames,
		listedAt(spring, springHand): springList,
		listedAt(autumn, autumnHand): autumnList}
	archive, err := keck.New(replay.Web(test, replies), most)
	if err != nil {
		test.Fatal(err)
	}
	return archive
}

// the test: the archive is named, asked by name only, and asks for at
// least one frame; it refuses none, and no web.
func TestTheArchiveIsAskedByName(test *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(test))
	archive, err := keck.New(get, least)
	if err != nil || archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%v, %v", archive, err)
	}
	refused, err := keck.New(get, none)
	if refused != nil || !errors.Is(err, celestialdata.Refused()) ||
		!strings.Contains(err.Error(), "at least one frame") {
		test.Errorf("most none: %v", err)
	}
	if nowhere, err := keck.New(nil, most); nowhere != nil || err == nil {
		test.Errorf("no web: %v", err)
	}
}

// fetched is a product as a search should give it.
type fetched struct{ id, name, address, about string }

// the test: a search KOA answers with two frames finds each frame's
// raw file, then its three MAKEE spectra, one a CCD, each where KOA
// serves it; a search round Sgr A*, where KOA has no public frame,
// finds none.
func TestASearchFindsEachFrameAndItsSpectra(test *testing.T) {
	archive := answering(test, "hires-two.xml",
		"level1-HI.20070426.25454.json", "level1-HI.20071123.53347.json")
	products, err := archive.Find(two(test))
	if err != nil {
		test.Fatal(err)
	}
	var wanted []fetched
	frames := []fetched{{spring, spring, springHand, springAbout},
		{autumn, autumn, autumnHand, autumnAbout}}
	for _, frame := range frames {
		wanted = append(wanted, fetched{frame.id, frame.id,
			rawAt(frame.address), frame.about})
		stem := strings.TrimSuffix(frame.id, ".fits")
		folder := strings.Replace(frame.address, "lev0/"+frame.id, "lev1/", once)
		for _, ccd := range []string{"1", "2", "3"} {
			file := stem + "_" + ccd + "_Flux.fits.gz"
			kept := folder + "makee/ccd" + ccd + "/fits/" + file
			wanted = append(wanted, fetched{frame.id, file,
				reducedAt(frame.id, kept), frame.about + extracted})
		}
	}
	if len(products) != len(wanted) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range wanted {
		product := products[at]
		if product.ID() != want.id || product.Name() != want.name ||
			product.URL() != want.address || product.About() != want.about ||
			product.Archive() != adapter {
			test.Errorf("%s %s %s %q", product.ID(), product.Name(),
				product.URL(), product.About())
		}
	}
	sgra := map[string]string{asked(test, "asked-sgra.txt"): "hires-sgra.xml"}
	south, err := keck.New(replay.Web(test, sgra), most)
	if err != nil {
		test.Fatal(err)
	}
	found, err := south.Find(replay.Asked(test))
	if found != nil || err != nil {
		test.Errorf("round Sgr A*: %v, %v", found, err)
	}
}

// the test: a frame that names its target only in object is named so;
// one KOA reduced none of is its raw file alone; and only the flux files
// of makee's fits folders are spectra.
func TestWhatAFrameHasIsFound(test *testing.T) {
	archive := answering(test, "crafted-object-only.xml",
		"crafted-level1-others.json", "crafted-level1-none.json")
	products, err := archive.Find(two(test))
	if err != nil {
		test.Fatal(err)
	}
	named := strings.Replace(springAbout, "of 85472", "of 509* 10700", once)
	kept := "/koadata2/HIRES/20070426/lev1/makee/ccd1/fits/" +
		"HI.20070426.25454_1_Flux.fits.gz"
	wanted := []fetched{{spring, spring, rawAt(springHand), named},
		{spring, "HI.20070426.25454_1_Flux.fits.gz", reducedAt(spring, kept),
			named + extracted},
		{autumn, autumn, rawAt(autumnHand), autumnAbout}}
	if len(products) != len(wanted) {
		test.Fatalf("%d products", len(products))
	}
	for at, want := range wanted {
		product := products[at]
		if product.ID() != want.id || product.Name() != want.name ||
			product.URL() != want.address || product.About() != want.about {
			test.Errorf("%s %s %s %q", product.ID(), product.Name(),
				product.URL(), product.About())
		}
	}
}

// the anti-test: a search the archive cannot read is refused, by its
// name, saying why: an answer that lacks a column, a frame whose id is
// no file name, a level-1 list that is not JSON, and one naming a file
// outside its folder.
func TestAnAnswerNotReadIsRefused(test *testing.T) {
	cases := []struct{ frames, list, said string }{
		{"crafted-no-progpi.xml", "level1-HI.20070426.25454.json", "progpi"},
		{"crafted-empty-koaid.xml", "", "plain"},
		{"hires-two.xml", "crafted-level1-not-json.json",
			"the level-1 list of " + spring},
		{"hires-two.xml", "crafted-level1-not-plain.json", "plain"}}
	for _, each := range cases {
		replies := map[string]string{asked(test, "asked-two.txt"): each.frames}
		if each.frames == "hires-two.xml" {
			replies[listedAt(spring, springHand)] = each.list
		}
		archive, err := keck.New(replay.Web(test, replies), most)
		if err != nil {
			test.Fatal(err)
		}
		found, err := archive.Find(two(test))
		if found != nil || err == nil || !strings.Contains(err.Error(),
			each.said) || !strings.HasPrefix(err.Error(), adapter) {
			test.Errorf("%s, %s: %v, %v", each.frames, each.list, found, err)
		}
	}
}

// the anti-test: a search the web cannot answer is refused, by the
// archive's name, whether it is the frames' question or a list's.
func TestASearchTheWebCannotAnswerIsRefused(test *testing.T) {
	near := two(test)
	unreachable := errors.New("no route to KOA")
	for _, failing := range []string{asked(test, "asked-two.txt"),
		listedAt(spring, springHand)} {
		get := mocks.NewMockWeb(gomock.NewController(test))
		if failing != asked(test, "asked-two.txt") {
			recorded, err := os.Open("testdata/hires-two.xml")
			if err != nil {
				test.Fatal(err)
			}
			get.EXPECT().Get(asked(test, "asked-two.txt")).Return(recorded,
				int64(none), nil)
		}
		get.EXPECT().Get(failing).Return(nil, int64(none), unreachable)
		archive, err := keck.New(get, most)
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
