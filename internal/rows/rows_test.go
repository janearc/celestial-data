package rows_test

import (
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial-data/internal/rows"
)

// The places and rows the tests ask about, each defined.
const (
	somewhereRA  = 266.4 // defined: degrees
	somewhereDec = -29.0 // defined: degrees
	pastThePole  = 91    // defined: degrees
	roundTrip    = 1e-14 // defined: a double through radians and back, as a share
	someRow      = 7     // defined: a row
)

// degrees are a row's two numbers for its place.
type degrees struct{ ascension, declination float64 }

// the test: a refusal names what it refuses and wraps both the
// package's refusal and why.
func TestARefusalWrapsWhy(test *testing.T) {
	why := errors.New("the reason")
	err := rows.Refusing(why, "row %d", someRow)
	if !errors.Is(err, celestialdata.Refused()) || !errors.Is(err, why) ||
		!strings.Contains(err.Error(), "row 7: the reason") {
		test.Errorf("refusing: %v", err)
	}
}

// the test: degrees on the sky are a place there, and both left empty
// are none.
func TestDegreesArePlaced(test *testing.T) {
	place, err := rows.Placed(somewhereRA, somewhereDec)
	if err != nil || place == nil {
		test.Fatalf("a place: %v, %v", place, err)
	}
	ascension := place.Ascension().InDegrees()
	declination := place.Declination().InDegrees()
	if math.Abs(ascension-somewhereRA) > roundTrip*somewhereRA ||
		math.Abs(declination-somewhereDec) > -roundTrip*somewhereDec {
		test.Errorf("placed at %g %g", ascension, declination)
	}
	none, err := rows.Placed(math.NaN(), math.NaN())
	if none != nil || err != nil {
		test.Errorf("no place: %v, %v", none, err)
	}
}

// the anti-test: a place past the pole is refused, by Place and Placed,
// and Placed refuses one empty number alone.
func TestAPlaceOffTheSkyIsRefused(test *testing.T) {
	refused := map[string]degrees{
		"past the pole":      {somewhereRA, pastThePole},
		"no declination":     {somewhereRA, math.NaN()},
		"no right ascension": {math.NaN(), somewhereDec}}
	for why, at := range refused {
		place, err := rows.Placed(at.ascension, at.declination)
		if place != nil || err == nil {
			test.Errorf("%s: %v, %v", why, place, err)
		}
	}
	place, err := rows.Place(somewhereRA, pastThePole)
	if place != nil || err == nil {
		test.Errorf("Place past the pole: %v, %v", place, err)
	}
}
