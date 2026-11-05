package skyview_test

import (
	"errors"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/replay"
	"github.com/janearc/celestial-data/skyview"
)

// What SkyView answers, as its batch query takes it. These are defined by
// SkyView's own query page.
const (
	query = "https://skyview.gsfc.nasa.gov/current/cgi/runquery.pl"
	// the survey SkyView calls this, and the file name it is given here
	haslam    = "408MHz"
	haslamOut = "skyview-408MHz-720.fits"
	// a survey whose name holds characters a file name should not
	spaced    = "Planck 857/GHz"
	spacedOut = "skyview-Planck-857-GHz-720.fits"
)

// The numbers the tests work with.
const (
	wide    = 720 // defined: a width for a map, in pixels
	tall    = 360 // defined: half the width, for the map's height
	lowest  = 2   // defined: the least width, since a map is half as tall
	toolow  = 1   // defined: a width below the least
	lone    = 1   // defined: one survey, one product, one pixel tall
	both    = 2   // defined: the surveys the archive is made of
	first   = 0   // defined: the first product
	second  = 1   // defined: the second product
	unsized = 0   // defined: the size of a file the archive does not size
)

// The sky's size in degrees, whatever the survey. It's defined by the
// whole of galactic longitude and latitude.
const sky = "360,180"

// The north galactic pole, far from where the other test asks. It's
// defined by the Hipparcos definition of the galactic frame.
const (
	poleRA  = 192.85948 // defined: the pole's right ascension
	poleDec = 27.12825  // defined: the pole's declination
	reach   = 0.1       // defined: the radius of the circle there
)

// archiveOf is the archive of the surveys at the wide width.
func archiveOf(t *testing.T, surveys ...string) *celestialdata.Fixed {
	t.Helper()
	held, err := skyview.New(wide, surveys...)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

// circleAt is a circle of the reach radius at a position.
func circleAt(t *testing.T, ra, dec float64) *celestialdata.Near {
	t.Helper()
	centre, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		t.Fatal(err)
	}
	near, err := celestialdata.NewNear(centre, reach)
	if err != nil {
		t.Fatal(err)
	}
	return near
}

// The archive is named for the command line, and is asked only by name.
func TestTheArchiveIsAskedOnlyByName(t *testing.T) {
	var asked celestialdata.ByName = archiveOf(t, haslam)
	if asked.Name() != "skyview" || !asked.ByNameOnly() {
		t.Errorf("%q, by name only %v", asked.Name(), asked.ByNameOnly())
	}
}

// Each survey is a map of its own, named for it, in the order given. Its
// query asks for the whole sky, in galactic coordinates, as FITS.
func TestTheArchiveHoldsAMapOfEachSurvey(t *testing.T) {
	held := archiveOf(t, haslam, spaced)
	found, err := held.Find(replay.Asked(t))
	if err != nil || len(found) != both {
		t.Fatalf("%d products, %v", len(found), err)
	}
	for place, want := range map[int]struct{ survey, file string }{
		first:  {haslam, haslamOut},
		second: {spaced, spacedOut},
	} {
		got := found[place]
		if got.Archive() != "skyview" || got.ID() != want.survey ||
			got.Name() != want.file ||
			got.About() != "SkyView whole-sky "+want.survey+
				"; its terms are on SkyView's survey page" {
			t.Errorf("product %d: %s %s %s %q", place, got.Archive(),
				got.ID(), got.Name(), got.About())
		}
		asked, err := url.Parse(got.URL())
		if err != nil {
			t.Fatal(err)
		}
		if asked.Scheme+"://"+asked.Host+asked.Path != query {
			t.Errorf("product %d asks %s", place, got.URL())
		}
		params := map[string]string{
			"Survey":      want.survey,
			"Projection":  "Car",
			"Coordinates": "Galactic",
			"Position":    "0,0",
			"Size":        sky,
			"Pixels":      pixels(wide, tall),
			"Return":      "FITS",
		}
		for param, value := range params {
			if asked.Query().Get(param) != value {
				t.Errorf("product %d: %s is %q, not %q", place, param,
					asked.Query().Get(param), value)
			}
		}
		if len(asked.Query()) != len(params) {
			t.Errorf("product %d asks %v", place, asked.Query())
		}
	}
}

// A file name keeps every letter and digit of a survey's name, up to the
// first and last of each kind, and makes every other character a hyphen.
// The characters around each kind are the ones next to it in ASCII.
func TestAFileNameKeepsLettersAndDigitsToTheirEnds(t *testing.T) {
	const (
		kept   = "AZaz09"
		around = "@[`{/:"
	)
	for survey, want := range map[string]string{
		kept:   "skyview-" + kept + "-720.fits",
		around: "skyview-" + strings.Repeat("-", len(around)) + "-720.fits",
	} {
		found, err := archiveOf(t, survey).Find(replay.Asked(t))
		if err != nil || len(found) != lone {
			t.Fatalf("%q: %d products, %v", survey, len(found), err)
		}
		if found[first].Name() != want {
			t.Errorf("%q is filed as %q, not %q", survey,
				found[first].Name(), want)
		}
	}
}

// pixels is a map's size in pixels, as the query writes it.
func pixels(across, down int) string {
	return strconv.Itoa(across) + "," + strconv.Itoa(down)
}

// SkyView does not say a map's size ahead, so none is held: a fetch of a
// few bytes is not turned away for its length.
func TestTheMapsAreUnsized(t *testing.T) {
	found, err := archiveOf(t, haslam).Find(replay.Asked(t))
	if err != nil || len(found) == unsized {
		t.Fatalf("%d products, %v", len(found), err)
	}
	if err := replay.Fetched(t, found[first]); err != nil {
		t.Errorf("a map with no size: %v", err)
	}
}

// The narrowest map is two pixels wide and one tall.
func TestTheNarrowestMapIsTwoPixelsWide(t *testing.T) {
	held, err := skyview.New(lowest, haslam)
	if err != nil {
		t.Fatal(err)
	}
	found, err := held.Find(replay.Asked(t))
	if err != nil || len(found) != lone {
		t.Fatalf("%d products, %v", len(found), err)
	}
	asked, err := url.Parse(found[first].URL())
	if err != nil || asked.Query().Get("Pixels") != pixels(lowest, lone) {
		t.Errorf("%s, %v", found[first].URL(), err)
	}
}

// Two circles far apart are given the same maps.
func TestFindIsTheSameForAnyCircle(t *testing.T) {
	held := archiveOf(t, haslam, spaced)
	here, err := held.Find(replay.Asked(t))
	if err != nil {
		t.Fatal(err)
	}
	there, err := held.Find(circleAt(t, poleRA, poleDec))
	if err != nil || len(there) != both || len(here) != both {
		t.Fatalf("%d and %d products, %v", len(here), len(there), err)
	}
	for place := range here {
		if *here[place] != *there[place] {
			t.Errorf("product %d differs", place)
		}
	}
}

// needs begins what a survey or a width that cannot be asked is told.
const needs = "skyview needs a survey and a width, not "

// width is a width as a refusal writes it.
func width(across int) string { return strconv.Itoa(across) }

// Anti-tests: no survey, a survey with no name, and a width below two are
// refused, and say so. The archive is none.
func TestNewRefusesWhatItCannotAsk(t *testing.T) {
	for _, attempt := range []struct {
		width   int
		surveys []string
		words   string
	}{
		{wide, nil, "skyview needs a survey"},
		{wide, []string{haslam, ""}, needs + `"" and ` + width(wide)},
		{toolow, []string{haslam},
			needs + `"` + haslam + `" and ` + width(toolow)},
		{-toolow, []string{haslam}, `and ` + width(-toolow)},
	} {
		held, err := skyview.New(attempt.width, attempt.surveys...)
		if !errors.Is(err, celestialdata.Refused()) || held != nil ||
			!strings.Contains(err.Error(), attempt.words) {
			t.Errorf("%d, %q: %v, %v", attempt.width, attempt.surveys,
				held, err)
		}
	}
}
