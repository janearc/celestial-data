package celestialdata_test

import (
	"errors"
	"math"
	"os"
	"testing"

	"github.com/janearc/celestial-data"
	"pgregory.net/rapid"
)

// The galactic frame by the Hipparcos definition. It is set out in ESA
// SP-1200 volume 1 section 1.5.3. Each number is defined.
const (
	// galacticPoleRA is the right ascension of the north galactic pole.
	// It's defined.
	galacticPoleRA = 192.85948
	// galacticPoleDec is the declination of the north galactic pole. It's
	// defined.
	galacticPoleDec = 27.12825
	// celestialPoleL is the galactic longitude of the north celestial
	// pole. It's defined.
	celestialPoleL = 122.93192
)

// The angles the sky tests work with, in degrees.
const (
	quarter  = 90   // defined: a quarter turn
	halfWay  = 180  // defined: half a turn
	tileHalf = 1.55 // defined: half a GLIMPSE II tile's 3.1 degrees
	tall     = 2.25 // defined: half the same tile's 4.5 degrees
	tileStep = 3    // defined: GLIMPSE II's tiles stand 3 degrees apart
	small    = 0.1  // defined: the radius asked for round Sgr A* that night
	poleward = 0.05 // defined: how far short of a pole the circle sits
)

// numbersFile is where the root package's test numbers are kept.
const numbersFile = "testdata/numbers.yaml"

// measured is the root package's test numbers, read by NewMeasured.
func measured(t *testing.T) *celestialdata.Measured {
	t.Helper()
	raw, err := os.ReadFile(numbersFile)
	if err != nil {
		t.Fatal(err)
	}
	return celestialdata.NewMeasured(numbersFile, raw)
}

// picked is a picked number from the test numbers.
func picked(t *testing.T, name string) float64 {
	t.Helper()
	value, err := measured(t).Get(name)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

// at is the place at a right ascension and declination, for a test.
func at(t *testing.T, ra, dec float64) *celestialdata.Position {
	t.Helper()
	place, err := celestialdata.NewPosition(ra, dec)
	if err != nil {
		t.Fatal(err)
	}
	return place
}

// lands is whether a place is at a galactic longitude and latitude to
// within rounding. Longitudes just short of a full turn are near none.
func lands(place celestialdata.Galactic, lon, lat, rounding float64) bool {
	across := math.Remainder(place.Longitude()-lon, raEnd)
	return math.Abs(across) <= rounding &&
		math.Abs(place.Latitude()-lat) <= rounding
}

// The points the definition itself fixes land where it puts them.
func TestGalacticLandsWhereTheDefinitionSays(t *testing.T) {
	rounding := picked(t, "rounding")
	pole := at(t, galacticPoleRA, galacticPoleDec).Galactic()
	if !(math.Abs(pole.Latitude()-northPole) <= rounding) {
		t.Errorf("the north galactic pole is at latitude %v", pole.Latitude())
	}
	for why, each := range map[string]struct {
		place    *celestialdata.Position
		lon, lat float64
	}{
		"the north celestial pole": {
			at(t, raStart, northPole), celestialPoleL, galacticPoleDec},
		"the ascending node": {at(t, galacticPoleRA+quarter, equator),
			celestialPoleL - quarter, equator},
		"the descending node": {at(t, galacticPoleRA-quarter, equator),
			celestialPoleL + quarter, equator},
	} {
		galactic := each.place.Galactic()
		if !lands(galactic, each.lon, each.lat, rounding) {
			t.Errorf("%s: %v, %v", why, galactic.Longitude(), galactic.Latitude())
		}
	}
}

// separation is the angle between two places in degrees. Its formula
// stays exact for small angles and for large ones.
func separation(lon, lat, otherLon, otherLat float64) float64 {
	toRadians := math.Pi / halfWay
	sinLat, cosLat := math.Sincos(lat * toRadians)
	sinOther, cosOther := math.Sincos(otherLat * toRadians)
	sinApart, cosApart := math.Sincos((otherLon - lon) * toRadians)
	east := cosOther * sinApart
	north := cosLat*sinOther - sinLat*cosOther*cosApart
	toward := sinLat*sinOther + cosLat*cosOther*cosApart
	return math.Atan2(math.Hypot(east, north), toward) / toRadians
}

// drawPlace draws a place on the sky.
func drawPlace(t *rapid.T, label string) *celestialdata.Position {
	ra := rapid.Float64Range(raStart, raEnd).Draw(t, label+" ra")
	dec := rapid.Float64Range(southPole, northPole).Draw(t, label+" dec")
	place, err := celestialdata.NewPosition(math.Mod(ra, raEnd), dec)
	if err != nil {
		t.Fatal(err)
	}
	return place
}

// keepsAngles is the property of TestGalacticKeepsEveryAngle, for one draw.
func keepsAngles(t *rapid.T) {
	one, other := drawPlace(t, "one"), drawPlace(t, "other")
	inICRS := separation(one.RA(), one.Dec(), other.RA(), other.Dec())
	near, far := one.Galactic(), other.Galactic()
	inGalactic := separation(near.Longitude(), near.Latitude(), far.Longitude(), far.Latitude())
	if !(math.Abs(inICRS-inGalactic) <= rounding) {
		t.Fatalf("%v degrees apart in ICRS, %v in galactic", inICRS,
			inGalactic)
	}
	if near.Longitude() < raStart || near.Longitude() >= raEnd ||
		near.Latitude() < southPole || near.Latitude() > northPole {
		t.Fatalf("off the sky: %v, %v", near.Longitude(), near.Latitude())
	}
}

// rounding is the test numbers' rounding for the properties. They have
// no *testing.T to read it with. TestMain fills it.
var rounding float64

func TestMain(suite *testing.M) {
	raw, err := os.ReadFile(numbersFile)
	if err == nil {
		rounding, err = celestialdata.NewMeasured(numbersFile, raw).
			Get("rounding")
	}
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(failed)
	}
	os.Exit(suite.Run())
}

// failed is the exit status of a test binary that could not begin. It's
// defined: any status but none is a failure.
const failed = 1

// Property: the galactic frame is a turn of the sky. So every angle
// between two places is the same in either frame.
func TestGalacticKeepsEveryAngle(t *testing.T) {
	rapid.Check(t, keepsAngles)
}

// patch is a box for a test.
func patch(t *testing.T, frame celestialdata.Frame, lon, lat, halfLon,
	halfLat float64) *celestialdata.Patch {
	t.Helper()
	box, err := celestialdata.NewPatch(frame, lon, lat, halfLon, halfLat)
	if err != nil {
		t.Fatal(err)
	}
	return box
}

// circle is a circle for a test.
func circle(t *testing.T, centre *celestialdata.Position,
	radius float64) *celestialdata.Near {
	t.Helper()
	near, err := celestialdata.NewNear(centre, radius)
	if err != nil {
		t.Fatal(err)
	}
	return near
}

// Circles on the galactic plane reach the tile they fall in and not the
// tiles beside it. The ascending node is at a longitude the definition
// fixes.
func TestACircleReachesTheTileItFallsIn(t *testing.T) {
	node := at(t, galacticPoleRA+quarter, equator)
	near := circle(t, node, small)
	at33 := celestialPoleL - quarter
	for lon, want := range map[float64]bool{
		math.Round(at33):            true,
		math.Round(at33) + tileStep: false,
		math.Round(at33) - tileStep: false,
	} {
		tile := patch(t, celestialdata.InGalactic, lon, equator, tileHalf, tall)
		if near.Reaches(tile) != want {
			t.Errorf("the tile at l %v: reached %v", lon, !want)
		}
	}
	far := patch(t, celestialdata.InGalactic, math.Round(at33), northPole-tall,
		tileHalf, tall)
	if near.Reaches(far) {
		t.Error("a tile far up the sky is reached")
	}
}

// Circles round a pole reach every longitude.
func TestACircleRoundAPoleReachesEveryLongitude(t *testing.T) {
	near := circle(t, at(t, raStart, northPole-poleward), small)
	for _, lon := range []float64{raStart, quarter, halfWay} {
		box := patch(t, celestialdata.InICRS, lon, northPole-tall, small, tall)
		if !near.Reaches(box) {
			t.Errorf("the box at %v is not reached", lon)
		}
	}
}

// insideReaches is the property of TestACircleInsideAPatchReachesIt, for
// one draw.
func insideReaches(t *rapid.T) {
	halfLat := rapid.Float64Range(pointlike, quarter/tileStep).
		Draw(t, "half latitude")
	lat := rapid.Float64Range(southPole+halfLat, northPole-halfLat).
		Draw(t, "latitude")
	halfLon := rapid.Float64Range(pointlike, quarter).Draw(t, "half")
	lon := rapid.Float64Range(raStart, raEnd).Draw(t, "longitude")
	box, err := celestialdata.NewPatch(celestialdata.InICRS, lon, lat, halfLon, halfLat)
	if err != nil {
		t.Fatal(err)
	}
	along := rapid.Float64Range(-unit, unit).Draw(t, "along")
	up := rapid.Float64Range(-unit, unit).Draw(t, "up")
	ra := math.Mod(lon+along*halfLon+raEnd, raEnd)
	centre, err := celestialdata.NewPosition(ra, lat+up*halfLat)
	if err != nil {
		t.Fatal(err)
	}
	radius := rapid.Float64Range(small, widest).Draw(t, "radius")
	near, err := celestialdata.NewNear(centre, radius)
	if err != nil {
		t.Fatal(err)
	}
	if !near.Reaches(box) {
		t.Fatal("a circle inside the box does not reach it")
	}
}

// unit is the most a drawn share of a reach is. It's defined.
const unit = 1

// Property: a circle whose centre is inside a patch reaches it, however
// small or large.
func TestACircleInsideAPatchReachesIt(t *testing.T) {
	rapid.Check(t, insideReaches)
}

// Anti-tests: NewPatch refuses a frame it does not know. It refuses a
// centre off the sky and a reach below nothing or not a number.
func TestNewPatchRefusesWhatIsNotABox(t *testing.T) {
	nan, inf := math.NaN(), math.Inf(positive)
	for why, each := range map[string]struct {
		frame                      celestialdata.Frame
		lon, lat, halfLon, halfLat float64
	}{
		"no frame":      {"", raStart, equator, small, small},
		"lon NaN":       {celestialdata.InICRS, nan, equator, small, small},
		"lon infinite":  {celestialdata.InICRS, inf, equator, small, small},
		"lat NaN":       {celestialdata.InICRS, raStart, nan, small, small},
		"lat past pole": {celestialdata.InICRS, raStart, widest, small, small},
		"lat below":     {celestialdata.InICRS, raStart, -widest, small, small},
		"half below":    {celestialdata.InICRS, raStart, equator, -small, small},
		"height below":  {celestialdata.InICRS, raStart, equator, small, -small},
		"half NaN":      {celestialdata.InICRS, raStart, equator, nan, small},
		"height NaN":    {celestialdata.InICRS, raStart, equator, small, inf},
	} {
		_, err := celestialdata.NewPatch(each.frame, each.lon, each.lat,
			each.halfLon, each.halfLat)
		if !errors.Is(err, celestialdata.Refused()) {
			t.Errorf("%s: %v", why, err)
		}
	}
}

// Boxes moved along the plane reach what is near their new centre. They
// grow by what they are given.
func TestAMovedBoxIsTheSameBoxElsewhere(t *testing.T) {
	node := at(t, galacticPoleRA+quarter, equator)
	near := circle(t, node, small)
	at33 := celestialPoleL - quarter
	far := patch(t, celestialdata.InGalactic, at33+halfWay, equator, tileHalf,
		tall)
	if near.Reaches(far) {
		t.Fatal("a box half a turn away is reached")
	}
	if !near.Reaches(far.Moved(at33, pointlike)) {
		t.Error("moved home, it is not reached")
	}
	if !near.Reaches(far.Moved(at33+tileStep, tileStep)) {
		t.Error("moved beside and grown, it is not reached")
	}
	if near.Reaches(far.Moved(at33+tileStep, -tileStep)) {
		t.Error("moved beside and shrunk, it is reached")
	}
	nowhere := far.Moved(math.NaN(), pointlike)
	if near.Reaches(nowhere) {
		t.Error("moved nowhere, it is reached")
	}
}

// Patches may sit on a pole and reach nothing either way. Their longitude
// must still be a number.
func TestAPatchsEdgesAreItsOwn(t *testing.T) {
	for _, lat := range []float64{southPole, northPole} {
		if _, err := celestialdata.NewPatch(celestialdata.InICRS, raStart, lat,
			pointlike, pointlike); err != nil {
			t.Errorf("at %v: %v", lat, err)
		}
	}
	_, err := celestialdata.NewPatch(celestialdata.InICRS, math.Inf(negative), equator,
		small, small)
	if !errors.Is(err, celestialdata.Refused()) {
		t.Errorf("at minus infinity: %v", err)
	}
}

// The numbers the edge tests are built on. Each is exact in binary, so
// the sums the tests make are exact too.
const (
	edgeHalf   = 1    // defined: a box's reach in latitude
	edgeRadius = 0.5  // defined: a circle's radius
	edgeDec    = 1.5  // defined: the reach and the radius together
	highDec    = 60   // defined: where a degree of right ascension is half one on the sky
	highHalf   = 10   // defined: a reach in latitude that holds the circle
	highBox    = 10   // defined: the box's right ascension
	highAhead  = 11.9 // defined: farther east of it than its reach and the radius
)

// Circles that just touch a box's edge in latitude reach it.
func TestACircleTouchingTheEdgeReachesIt(t *testing.T) {
	box := patch(t, celestialdata.InICRS, raStart, equator, edgeHalf, edgeHalf)
	near := circle(t, at(t, raStart, edgeDec), edgeRadius)
	if !near.Reaches(box) {
		t.Error("a circle on the edge is not reached")
	}
}

// Far from the equator circles span more degrees of right ascension than
// their radius. One of a degree at declination 60 spans about two either
// side. So it reaches a box at 10 from 11.9, though the box reaches only
// to 11.
func TestACircleSpansMoreLongitudeNearAPole(t *testing.T) {
	box := patch(t, celestialdata.InICRS, highBox, highDec, edgeHalf, highHalf)
	near := circle(t, at(t, highAhead, highDec), edgeHalf)
	if !near.Reaches(box) {
		t.Error("the circle falls short of the box")
	}
}

// Rounding can push the latitude's sine past one a hair from the pole.
// The latitude there is the pole's, not NaN.
func TestASinePastOneIsThePole(t *testing.T) {
	place := at(t, picked(t, "past-one-ra"), picked(t, "past-one-dec"))
	lat := place.Galactic().Latitude()
	if !(math.Abs(lat-northPole) <= rounding) {
		t.Errorf("latitude %v", lat)
	}
}
