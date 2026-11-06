package galah_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/galah"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
)

// The archive's numbers, each defined.
const (
	adapter      = "galah"       // defined: the archive's name
	lone         = 1             // defined: the one file
	first        = 0             // defined: its place
	allStarBytes = 757998720     // defined: numbers.yaml's allstar-size
	shortBody    = "a few bytes" // defined: a body shorter than the file
)

// the test: the archive is asked by name only, and holds the one file,
// DR4's allstar at Data Central, whatever the circle.
func TestTheArchiveIsTheAllstarFile(test *testing.T) {
	archive, err := galah.New()
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
	if allStar.Archive() != adapter || allStar.ID() != "dr4 allstar" ||
		allStar.Name() != "galah_dr4_allstar_240705.fits" ||
		allStar.URL() != "https://cloud.datacentral.org.au/teamdata/GALAH/"+
			"public/GALAH_DR4/catalogs/galah_dr4_allstar_240705.fits" ||
		!strings.Contains(allStar.About(), "GALAH DR4") {
		test.Errorf("%s %s %s %s %q", allStar.Archive(), allStar.ID(),
			allStar.Name(), allStar.URL(), allStar.About())
	}
}

// the anti-test: a fetch that brings fewer bytes than numbers.yaml says
// the file holds is refused, saying both lengths.
func TestAShortFetchOfTheFileIsRefused(test *testing.T) {
	archive, err := galah.New()
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
