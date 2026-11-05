package bluemarble_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/bluemarble"
	"github.com/janearc/celestial-data/internal/replay"
)

// What the archive holds, as the package says it. These are defined by
// NASA's Visible Earth, which serves the files under these addresses.
const (
	julyID    = "2004-07"
	julyName  = "world.200407.3x5400x2700.jpg"
	julyURL   = folder + "74000/74092/" + julyName
	julyAbout = "NASA Blue Marble Next Generation, July 2004; " +
		"fetched locally, never shipped"
	nightID    = "2016-night"
	nightName  = "BlackMarble_2016_01deg.jpg"
	nightURL   = folder + "144000/144898/" + nightName
	nightAbout = "NASA Black Marble 2016, the earth at night; " +
		"fetched locally, never shipped"
	folder = "https://eoimages.gsfc.nasa.gov/images/imagerecords/"
)

// The counts and places the tests work with.
const (
	images = 2 // defined: the July image and the night image
	july   = 0 // defined: the first product
	night  = 1 // defined: the second product
)

// The north galactic pole, far from where the other test asks. It's
// defined by the Hipparcos definition of the galactic frame.
const (
	poleRA  = 192.85948 // defined: the pole's right ascension
	poleDec = 27.12825  // defined: the pole's declination
	wide    = 0.1       // defined: the radius of the circle there
)

// lengths is the numbers file beside the package, read.
func lengths(t *testing.T) *celestialdata.Measured {
	t.Helper()
	raw, err := os.ReadFile("numbers.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return celestialdata.NewMeasured("numbers.yaml", raw)
}

// sizeOf is a length from the numbers file beside the package.
func sizeOf(t *testing.T, name string) int64 {
	t.Helper()
	length, err := lengths(t).Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return int64(length)
}

// archiveOf is the archive New gives.
func archiveOf(t *testing.T) *celestialdata.Fixed {
	t.Helper()
	held, err := bluemarble.New()
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// circleAt is a circle of the wide radius at a position.
func circleAt(t *testing.T, ra, dec float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, wide)
	if err != nil {
		t.Fatal(err)
	}
	return near
}

// The archive is named for the command line, and is asked only by name.
func TestTheArchiveIsAskedOnlyByName(t *testing.T) {
	held := archiveOf(t)
	var asked celestialdata.ByName = held
	if asked.Name() != "bluemarble" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
}

// The archive holds the two images, with their addresses, what they are
// and the lengths in the numbers file.
func TestTheArchiveHoldsTheTwoImages(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil || len(found) != images {
		t.Fatalf("%d products, %v", len(found), err)
	}
	for place, want := range map[int]struct{ id, name, url, about string }{
		july:  {julyID, julyName, julyURL, julyAbout},
		night: {nightID, nightName, nightURL, nightAbout},
	} {
		got := found[place]
		if got.Archive() != "bluemarble" || got.ID() != want.id ||
			got.Name() != want.name || got.URL() != want.url ||
			got.About() != want.about {
			t.Errorf("product %d: %s %s %s %s %q", place, got.Archive(),
				got.ID(), got.Name(), got.URL(), got.About())
		}
	}
}

// The lengths are held. No question goes to the network: the test fetches
// from a mock, which gives a few bytes, and the fetch says what length it
// was told to expect.
func TestTheImagesHoldTheLengthsOfTheNumbersFile(t *testing.T) {
	found, err := archiveOf(t).Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	for place, name := range map[int]string{july: "july-size",
		night: "night-size"} {
		want := fmt.Sprintf("the archive said %d", sizeOf(t, name))
		err := replay.Fetched(t, found[place])
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, not %q", name, err, want)
		}
	}
}

// Two circles far apart are given the same images, and what is given can
// be changed without changing what the archive holds.
func TestFindIsTheSameForAnyCircle(t *testing.T) {
	held := archiveOf(t)
	first, err := held.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := held.Find(circleAt(t, poleRA, poleDec))
	if err != nil || len(second) != len(first) {
		t.Fatalf("%d products, %v", len(second), err)
	}
	for place := range first {
		if *first[place] != *second[place] {
			t.Errorf("product %d differs", place)
		}
	}
}

// Every number the code asks the numbers file for is in it: Value panics
// on one it lacks, so New running through is the check.
func TestEveryNumberTheCodeAsksForIsInTheFile(t *testing.T) {
	if _, err := bluemarble.New(); err != nil {
		t.Errorf("New: %v", err)
	}
}
