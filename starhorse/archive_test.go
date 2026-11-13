package starhorse_test

import (
	"io"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial-data/starhorse"
)

// The archive's numbers, each defined.
const (
	adapter      = "starhorse"   // defined: the archive's name
	lone         = 1             // defined: the one file
	first        = 0             // defined: its place
	resultsBytes = 248604480     // defined: numbers.yaml's starhorse-size
	shortBody    = "a few bytes" // defined: a body shorter than the file
)

// the test: the archive is asked by name only, and holds the one file,
// StarHorse's APOGEE DR17 results at AIP, whatever the
// circle.
func TestTheArchiveIsTheStarHorseFile(test *testing.T) {
	archive, err := starhorse.New()
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
	if file.Archive() != adapter || file.ID() != "dr17 starhorse v2" ||
		file.Name() != "APOGEE_DR17_EDR3_StarHorse_v2.fits" ||
		file.URL() != "https://s3.data.aip.de:9000/shaqueiroz2023/"+
			"APOGEE_DR17_EDR3_StarHorse_v2.fits" ||
		!strings.Contains(file.About(), "masses, distances, extinctions, ages") {
		test.Errorf("%s %s %s %s %q", file.Archive(), file.ID(), file.Name(),
			file.URL(), file.About())
	}
}

// the anti-test: a fetch that brings fewer bytes than numbers.yaml says
// the file holds is refused, saying both lengths.
func TestAShortFetchOfTheFileIsRefused(test *testing.T) {
	archive, err := starhorse.New()
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
		strconv.Itoa(resultsBytes)
	if err == nil || !strings.Contains(err.Error(), want) {
		test.Errorf("a short fetch: %v", err)
	}
}
