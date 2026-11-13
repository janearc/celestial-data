package usno_test

import (
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/usno"
)

// What the archive holds, as the package says it. The address is the
// observatory's, where it serves the page.
const (
	sunID      = "approximate solar coordinates"
	sunName    = "sun-approx.html"
	sunAddress = "https://aa.usno.navy.mil/faq/sun_approx"
	sunAbout   = "USNO's approximate solar coordinates, a US government work"
	pages      = 1 // defined: the solar coordinates page
	sun        = 0 // defined: its place among the products
)

// archiveOf is the archive New gives.
func archiveOf(t *testing.T) *celestialdata.Fixed {
	t.Helper()
	held, err := usno.New()
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// the test: the archive is named for the command line, and is asked only
// by name.
func TestTheArchiveIsAskedOnlyByName(t *testing.T) {
	var asked celestialdata.ByName = archiveOf(t)
	if asked.Name() != "usno" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
}

// the test: the archive holds the solar coordinates page, by the
// observatory's address, kept as one html file.
func TestTheArchiveHoldsTheSolarCoordinates(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil || len(found) != pages {
		t.Fatalf("%d products, %v", len(found), err)
	}
	got := found[sun]
	if got.Archive() != "usno" || got.ID() != sunID ||
		got.Name() != sunName || got.URL() != sunAddress ||
		got.About() != sunAbout {
		t.Errorf("%s %s %s %s %q", got.Archive(), got.ID(), got.Name(),
			got.URL(), got.About())
	}
}

// the test: the page is fetched whole through the web it is given, with
// no length expected ahead, since the observatory says none. No question
// goes to the network: replay's mock gives a few bytes.
func TestThePageIsFetchedWithNoLengthAhead(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, found[sun]); err != nil {
		t.Errorf("the page was not fetched: %v", err)
	}
}
