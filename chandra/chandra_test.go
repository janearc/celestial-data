package chandra_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/chandra"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/tap"
	"go.uber.org/mock/gomock"
)

// The places the recorded replies came from.
const (
	heasarc = "https://heasarc.gsfc.nasa.gov/xamin/vo/tap/sync"
	tree    = "https://cxc.cfa.harvard.edu/cdaftp/byobsid"
)

// The counts the recorded replies were asked for and hold.
const (
	most    = 5  // defined: the TOP the recorded query asked for
	files   = 31 // defined: the FITS files the five primary folders list
	none    = 0  // defined: a search that asks for nothing
	unsized = 0  // defined: the size of a file the archive does not size
)

// observations are the five the recorded reply names, longest first.
var observations = []string{"13842", "13849", "13839", "3392", "13840"}

// asked is the address the adapter asks HEASARC, for the asked circle.
func asked(t *testing.T) string {
	adql := "SELECT TOP 5 obsid, name, exposure, ra, dec, detector, " +
		"grating, status FROM chanmaster WHERE " +
		tap.Within(replay.Asked(t), "ra", "dec") +
		" AND status = 'archived' ORDER BY exposure DESC"
	return tap.URL(heasarc, adql, "votable")
}

// folder is the address of an observation's primary folder.
func folder(obsid, last string) string {
	return fmt.Sprintf("%s/%s/%s/primary/", tree, last, obsid)
}

// recorded is every reply the archives gave that night, by address.
func recorded(t *testing.T) map[string]string {
	replies := map[string]string{asked(t): "chanmaster-sgra.xml"}
	for _, obsid := range observations {
		last := obsid[len(obsid)-len("2"):]
		replies[folder(obsid, last)] = "primary-" + obsid + ".html"
	}
	return replies
}

// archiveOf is Chandra asked through a web that gives recorded replies.
func archiveOf(t *testing.T, replies map[string]string) *chandra.Archive {
	t.Helper()
	archive, err := chandra.New(replay.Web(t, replies), most)
	if err != nil {
		t.Fatal(err)
	}
	return archive
}

// Each of the five longest observations of Sgr A* brings the FITS files
// in its primary folder, and nothing else the folder lists.
func TestFindNamesEachObservationsPrimaryFiles(t *testing.T) {
	archive := archiveOf(t, recorded(t))
	products, err := archive.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(products) != files {
		t.Fatalf("%d products", len(products))
	}
	want := folder("3392", "2") + "acisf03392N004_evt2.fits.gz"
	for _, product := range products {
		if product.URL() == want {
			if product.ID() != "3392" ||
				product.Archive() != archive.Name() ||
				product.About() != "SGR A*, ACIS-I with NONE, 168900 s" {
				t.Errorf("%s %s %q", product.ID(), product.Archive(),
					product.About())
			}
			return
		}
	}
	t.Errorf("no %s", want)
}

// Links out of the folder are not its products. Nor are files that are
// not FITS.
func TestFindKeepsOnlyTheFoldersFITSFiles(t *testing.T) {
	archive := archiveOf(t, map[string]string{
		asked(t):            "one-observation.xml",
		folder("3392", "2"): "primary-crafted.html",
	})
	products, err := archive.Find(replay.Asked(t))
	if err != nil || len(products) != len("1") ||
		products[none].Name() != "acisf03392N004_evt2.fits.gz" {
		t.Errorf("%v, %v", products, err)
	}
}

// Anti-tests: New refuses no web and a search for no observations.
func TestNewRefusesWhatCannotSearch(t *testing.T) {
	get := mocks.NewMockWeb(gomock.NewController(t))
	if _, err := chandra.New(get, none); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no observations: %v", err)
	}
	if _, err := chandra.New(nil, most); !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("no web: %v", err)
	}
}

// Anti-tests: a reply chanmaster should never send is an error, and so
// is a question nobody answers.
func TestFindRefusesWhatIsNotAnObservation(t *testing.T) {
	for _, file := range []string{"not-an-id.xml", "no-grating.xml"} {
		archive := archiveOf(t, map[string]string{asked(t): file})
		if _, err := archive.Find(replay.Asked(t)); err == nil {
			t.Errorf("%s: found", file)
		}
	}
}

// Anti-test: a search HEASARC does not answer is an error that says why.
func TestFindReportsAnUnansweredSearch(t *testing.T) {
	down := errors.New("down")
	get := mocks.NewMockWeb(gomock.NewController(t))
	get.EXPECT().Get(asked(t)).Return(nil, int64(unsized), down)
	archive, err := chandra.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Find(replay.Asked(t)); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}

// Anti-test: a primary folder that will not list is an error that says
// why.
func TestFindReportsAFolderThatWillNotList(t *testing.T) {
	down := errors.New("down")
	get := replay.Web(t, map[string]string{asked(t): "one-observation.xml"})
	get.EXPECT().Get(folder("3392", "2")).Return(nil, int64(unsized), down)
	archive, err := chandra.New(get, most)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := archive.Find(replay.Asked(t)); !errors.Is(err, down) {
		t.Errorf("%v", err)
	}
}

func TestTheAdapterIsCalledChandra(t *testing.T) {
	if name := archiveOf(t, nil).Name(); name != "chandra" {
		t.Errorf("named %s", name)
	}
}

// The files found come whole: each is of the size the adapter gave it.
// Searches may ask for one observation, the least.
func TestFoundFilesComeWholeAndOneIsEnough(t *testing.T) {
	products, err := archiveOf(t, recorded(t)).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, products[none]); err != nil {
		t.Errorf("%v", err)
	}
	get := mocks.NewMockWeb(gomock.NewController(t))
	if _, err := chandra.New(get, least); err != nil {
		t.Errorf("one observation: %v", err)
	}
}

// least is the fewest observations a search may ask for. It's defined.
const least = 1

// Ids count up from one: the first observation is asked for, and none
// numbered nothing is.
func TestIdsCountFromOne(t *testing.T) {
	first := archiveOf(t, map[string]string{asked(t): "first-id.xml",
		folder("1", "1"): "primary-crafted.html"})
	if products, err := first.Find(replay.Asked(t)); err != nil ||
		len(products) == none {
		t.Errorf("observation 1: %v, %v", products, err)
	}
	zero := archiveOf(t, map[string]string{asked(t): "zero-id.xml"})
	if _, err := zero.Find(replay.Asked(t)); err == nil {
		t.Error("observation 0 was asked for")
	}
}
