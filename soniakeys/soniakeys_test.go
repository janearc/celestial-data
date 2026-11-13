package soniakeys_test

import (
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/soniakeys"
)

// What the archive holds, as the package says it. The address pins the
// commit, so a moved file is a failed fetch, not a different file.
const (
	solarID      = "solar test"
	solarName    = "soniakeys-meeus-solar-test.go.txt"
	solarAddress = "https://raw.githubusercontent.com/soniakeys/meeus/" +
		"bfbd9ac2c7f709c94f1dee190c633d24a723f628/v3/solar/solar_test.go"
	files = 1 // defined: the solar test
	solar = 0 // defined: its place among the products
)

// archiveOf is the archive New gives.
func archiveOf(t *testing.T) *celestialdata.Fixed {
	t.Helper()
	held, err := soniakeys.New()
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// the test: the archive is named for the command line, and is asked only
// by name.
func TestTheArchiveIsAskedOnlyByName(t *testing.T) {
	var asked celestialdata.ByName = archiveOf(t)
	if asked.Name() != "soniakeys" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
}

// the test: the archive holds the solar test at the cited commit, kept
// as a .txt, and says it is for checking, not deriving.
func TestTheArchiveHoldsTheSolarTest(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil || len(found) != files {
		t.Fatalf("%d products, %v", len(found), err)
	}
	got := found[solar]
	if got.Archive() != "soniakeys" || got.ID() != solarID ||
		got.Name() != solarName || got.URL() != solarAddress ||
		!strings.Contains(got.About(), "not to derive from") {
		t.Errorf("%s %s %s %s %q", got.Archive(), got.ID(), got.Name(),
			got.URL(), got.About())
	}
}

// the test: the file is fetched whole through the web it is given, with
// no length expected ahead. No question goes to the network.
func TestTheFileIsFetchedWithNoLengthAhead(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := replay.Fetched(t, found[solar]); err != nil {
		t.Errorf("the file was not fetched: %v", err)
	}
}
