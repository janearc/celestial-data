package apogee_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/apogee"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
)

// The archive's numbers, each defined.
const (
	adapter      = "apogee"      // defined: the archive's name
	lone         = 1             // defined: the one file
	first        = 0             // defined: its place
	allStarBytes = 3970621440    // defined: numbers.yaml's allstar-size
	shortBody    = "a few bytes" // defined: a body shorter than the file
)

// the test: the archive is asked by name only, and holds the one file,
// DR17's allStar at the Science Archive Server, whatever the circle.
func TestTheArchiveIsTheAllStarFile(test *testing.T) {
	archive, err := apogee.New()
	if err != nil {
		test.Fatal(err)
	}
	if archive.Name() != adapter || !archive.ByNameOnly() {
		test.Errorf("%q, by name only %v", archive.Name(), archive.ByNameOnly())
	}
	found, err := archive.Find(replay.Asked(test))
	if err != nil || len(found) != lone {
		test.Fatalf("%d products, %v", len(found), err)
	}
	allStar := found[first]
	if allStar.Archive() != adapter || allStar.ID() != "dr17 allStar" ||
		allStar.Name() != "allStar-dr17-synspec_rev1.fits" ||
		allStar.URL() != "https://data.sdss.org/sas/dr17/apogee/spectro/"+
			"aspcap/dr17/synspec_rev1/allStar-dr17-synspec_rev1.fits" ||
		!strings.Contains(allStar.About(), "APOGEE DR17") {
		test.Errorf("%s %s %s %s %q", allStar.Archive(), allStar.ID(),
			allStar.Name(), allStar.URL(), allStar.About())
	}
}

// the anti-test: a fetch that brings fewer bytes than numbers.yaml says
// the file holds is refused, saying both lengths.
func TestAShortFetchOfTheFileIsRefused(test *testing.T) {
	archive, err := apogee.New()
	if err != nil {
		test.Fatal(err)
	}
	found, err := archive.Find(replay.Asked(test))
	if err != nil || len(found) != lone {
		test.Fatalf("%d products, %v", len(found), err)
	}
	get := mocks.NewMockWeb(gomock.NewController(test))
	reply := io.NopCloser(strings.NewReader(shortBody))
	promised := int64(len(shortBody))
	get.EXPECT().Get(found[first].URL()).Return(reply, promised, nil)
	_, err = celestialdata.Fetch(get, found[first], test.TempDir())
	want := strconv.Itoa(len(shortBody)) + " bytes came, the archive said " +
		strconv.Itoa(allStarBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		test.Errorf("a short fetch: %v", err)
	}
}
