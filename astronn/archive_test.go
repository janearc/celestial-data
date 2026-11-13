package astronn_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/astronn"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
)

// The archive's numbers, each defined.
const (
	adapter      = "astronn"     // defined: the archive's name
	lone         = 1             // defined: the one file
	first        = 0             // defined: its place
	astroNNBytes = 647328960     // defined: numbers.yaml's astronn-size
	shortBody    = "a few bytes" // defined: a body shorter than the file
)

// the test: the archive is asked by name only, and holds the one file,
// astroNN's DR17 catalogue at the Science Archive Server, whatever the
// circle.
func TestTheArchiveIsTheAstroNNFile(test *testing.T) {
	archive, err := astronn.New()
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
	file := found[first]
	if file.Archive() != adapter || file.ID() != "dr17 astroNN" ||
		file.Name() != "apogee_astroNN-DR17.fits" ||
		file.URL() != "https://data.sdss.org/sas/dr17/env/APOGEE_ASTRO_NN/"+
			"apogee_astroNN-DR17.fits" ||
		!strings.Contains(file.About(), "ages, distances and orbits") {
		test.Errorf("%s %s %s %s %q", file.Archive(), file.ID(), file.Name(),
			file.URL(), file.About())
	}
}

// the anti-test: a fetch that brings fewer bytes than numbers.yaml says
// the file holds is refused, saying both lengths.
func TestAShortFetchOfTheFileIsRefused(test *testing.T) {
	archive, err := astronn.New()
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
		strconv.Itoa(astroNNBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		test.Errorf("a short fetch: %v", err)
	}
}
