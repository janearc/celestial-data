package nustar_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/nustar"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// heasarc is HEASARC's sync endpoint, which the recorded reply came from.
const heasarc = "https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync"

// The counts the recorded reply was asked for and holds.
const (
	most    = 5  // defined: the TOP the recorded query asked for
	files   = 10 // defined: two modules' files for each of five
	none    = 0  // defined: a search that asks for nothing
	unsized = 0  // defined: the size of a file the archive does not size
	firstAt = 0  // defined: the first product, module A of the longest
	nextAt  = 1  // defined: the second product, module B of the longest
)

// asked is the address the adapter asks HEASARC, for the asked circle.
func asked(t *testing.T) string {
	adql := "SELECT TOP 5 obsid, name, exposure_a, ra, dec FROM numaster " +
		"WHERE " + tap.Within(replay.Asked(t), "ra", "dec") +
		" AND exposure_a > 0 ORDER BY exposure_a DESC"
	return tap.URL(heasarc, adql, "votable")
}

// answering is NuSTAR asked through a web that gives one recorded reply.
func answering(t *testing.T, file string) *nustar.Archive {
	t.Helper()
	get := replay.Web(t, map[string]string{asked(t): file})
	archive, err := nustar.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

// Each of the five longest observations of Sgr A* brings its two
// modules' event files, filed where HEASARC files them.
func TestFindNamesBothModulesOfEachObservation(t *testing.T) {
	archive := answering(t, "numaster-sgra.xml")
	products, err := archive.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != files {
		t.Fatalf("%d products", len(products))
	}
	first, second := products[firstAt], products[nextAt]
	want := "https://heasarc.gsfc.nasa.gov/FTP/nustar/data/obs/00/3/" +
		"30001002001/event_cl/nu30001002001A01_cl.evt.gz"
	if first.URL() != want || first.Name() != "nu30001002001A01_cl.evt.gz" ||
		first.ID() != "30001002001" || first.Archive() != archive.Name() ||
		first.About() != "SgrAstar, module A, 156789 s" {
		t.Errorf("%s %s %s %s %q", first.URL(), first.Name(), first.ID(),
			first.Archive(), first.About())
	}
	if !strings.HasSuffix(second.URL(), "/nu30001002001B01_cl.evt.gz") {
		t.Errorf("the second file is %s", second.URL())
	}
}

// Anti-tests: New refuses no web and a search for no observations.
func TestNewRefusesWhatCannotSearch(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	if _, err := nustar.New(get, none); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no observations: %v", err)
	}
	if _, err := nustar.New(nil, most); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
}

// Anti-tests: a reply numaster should never send is an error, and so is
// a search HEASARC does not answer.
func TestFindRefusesWhatIsNotAnObservation(t *testing.T) {
	for _, file := range []string{"short-obsid.xml", "slashed-obsid.xml",
		"no-name.xml"} {
		if _, err := answering(t, file).Find(replay.Asked(t)); err == nil {
			t.Errorf("%s: found", file)
		}
	}
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(asked(t)).Return(nil, int64(unsized), down)
	archive, err := nustar.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Find(replay.Asked(t)); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}

// The files found come whole: each is of the size the adapter gave it.
// Searches may ask for one observation, the least.
func TestFoundFilesComeWholeAndOneIsEnough(t *testing.T) {
	products, err := answering(t, "numaster-sgra.xml").Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, products[firstAt]); err != nil {
		t.Errorf("%v", err)
	}
	get := mocks.NewMockWeb(gomock.NewController(t))
	if _, err := nustar.New(get, least); err != nil {
		t.Errorf("one observation: %v", err)
	}
}

// least is the fewest observations a search may ask for. It's defined.
const least = 1
