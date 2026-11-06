package gaia_test

import (
	"math"
	"strings"
	"testing"

	"go.uber.org/mock/gomock"
	"pgregory.net/rapid"

	"github.com/janearc/celestial-data/gaia"
	"github.com/janearc/celestial-data/mocks"
	"github.com/janearc/celestial/galactocentric"
	"github.com/janearc/celestial/measure"
	"github.com/janearc/celestial/orbit"
)

// The draws tests' own numbers, each defined.
const (
	unitDeviate   = 1.0        // defined: one standard deviation
	noDeviate     = 0.0        // defined: no deviation
	sixDraws      = 6          // defined: five parameters and the radial velocity
	radialDraw    = 5          // defined: the radial velocity's, after the five
	nowhere       = -1         // defined: a place no draw has
	arcsecInDeg   = 3600       // defined: arcseconds in a degree
	degreesInTurn = 360        // defined: degrees in a turn
	raDec         = 0          // defined: ra_dec_corr's place in the row
	raParallax    = 1          // defined: ra_parallax_corr's place
	raPMRA        = 2          // defined: ra_pmra_corr's place
	raPMDec       = 3          // defined: ra_pmdec_corr's place
	decParallax   = 4          // defined: dec_parallax_corr's place
	strong        = 0.9        // defined: a correlation near one
	nearThePole   = 89.9999999 // defined: degrees, 0.36 mas short of it
	wideDecError  = 10         // defined: mas, past the pole in one step
	anyWeight     = 1.0        // defined: a drawn matrix's entries
	margin        = 0.1        // defined: down the diagonal, past rounding
	tightRoom     = 1e-9       // defined: a triangle's product, to rounding
)

// reporting is a test or a property's draw that a mock reports to.
type reporting interface {
	stopping
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// fiveParameters are the five astrometric parameters, in the archive's
// order.
func fiveParameters() []gaia.Parameter {
	return []gaia.Parameter{gaia.RA, gaia.Dec, gaia.Parallax, gaia.PMRA,
		gaia.PMDec}
}

// unitAt is a mocked normal whose six draws, in order, are nothing but
// at one place, where it is one standard deviation: the five parameters
// in the archive's order, then the radial velocity.
func unitAt(test reporting, place int) *mocks.MockNormal {
	return deviateAt(test, place, unitDeviate)
}

// deviateAt is a mocked normal whose six draws, in order, are nothing
// but at one place, where it is the deviate given.
func deviateAt(test reporting, place int,
	deviate float64) *mocks.MockNormal {
	normal := mocks.NewMockNormal(gomock.NewController(test))
	var calls []any
	for each := range sixDraws {
		value := noDeviate
		if each == place {
			value = deviate
		}
		calls = append(calls, normal.EXPECT().NormFloat64().Return(value))
	}
	gomock.InOrder(calls...)
	return normal
}

// spread is a source's spread, or the test stops.
func spread(test stopping, source *gaia.Source) *gaia.Spread {
	test.Helper()
	made, err := source.Spread()
	if err != nil {
		test.Fatal(err)
	}
	return made
}

// drawnBy is a spread's draw from the normal given, or the test stops.
func drawnBy(test stopping, made *gaia.Spread,
	normal gaia.Normal) *gaia.Source {
	test.Helper()
	drawn, err := made.Drawn(normal)
	if err != nil {
		test.Fatal(err)
	}
	return drawn
}

// moves are how far a draw moved each parameter, in milliarcseconds and
// milliarcseconds a year, the ascension along the sky.
type moves [fiveOfThem]float64

// shifts are how far a draw moved the source.
func shifts(source, drawn *gaia.Source) moves {
	cosine := math.Cos(source.Place().Declination().InRadians())
	round := math.Remainder(drawn.Place().Ascension().InDegrees()-
		source.Place().Ascension().InDegrees(), degreesInTurn)
	up := drawn.Place().Declination().InDegrees() -
		source.Place().Declination().InDegrees()
	return moves{round * cosine * arcsecInDeg * masInArcsecond,
		up * arcsecInDeg * masInArcsecond,
		masOf(drawn.Parallax()) - masOf(source.Parallax()),
		drawn.Motion().Ascension() - source.Motion().Ascension(),
		drawn.Motion().Declination() - source.Motion().Declination()}
}

// the test: a draw of no deviation in anything is the source itself.
func TestNoDeviatesDrawTheSourceItself(test *testing.T) {
	source := theSource(test)
	drawn := drawnBy(test, spread(test, source), unitAt(test, nowhere))
	same := drawn.Place() == source.Place() &&
		drawn.Parallax() == source.Parallax() &&
		drawn.Motion() == source.Motion() &&
		drawn.RadialVelocity() == source.RadialVelocity()
	if !same || drawn.ID() != source.ID() {
		test.Errorf("no deviates drew %+v from %+v", drawn, source)
	}
}

// the test: one standard deviation in the ascension moves each
// parameter by its error times its correlation with the ascension, the
// triangle's first column worked by hand from the made-up star; and one
// in the radial velocity, the sixth draw, moves it by its error alone.
func TestADeviateMovesWhatItIsCorrelatedWith(test *testing.T) {
	source := theSource(test)
	made := spread(test, source)
	ra := int(gaia.RA)
	normal := unitAt(test, ra)
	moved := shifts(source, drawnBy(test, made, normal))
	corrs := theStar().corrs
	wants := map[string]reading{
		"ra":  {moved[gaia.RA], madeRAError},
		"dec": {moved[gaia.Dec], madeDecError * float64(corrs[raDec])},
		"parallax": {moved[gaia.Parallax],
			madeParallaxErr * float64(corrs[raParallax])},
		"pmra":  {moved[gaia.PMRA], madePMRAError * float64(corrs[raPMRA])},
		"pmdec": {moved[gaia.PMDec], madePMDecError * float64(corrs[raPMDec])},
	}
	for name, each := range wants {
		if !near(each.got, each.want, singleRoom) {
			test.Errorf("%s moved %g, want %g", name, each.got, each.want)
		}
	}
	radial := drawnBy(test, made, unitAt(test, radialDraw))
	if got := radial.RadialVelocity() - source.RadialVelocity(); !near(got,
		madeRadialError, singleRoom) {
		test.Errorf("the radial velocity moved %g, want %g", got,
			madeRadialError)
	}
}

// the property: the triangle a spread draws by, times its transpose, is
// the covariance the source's errors and correlations make, for any
// covariance that is positive definite: each parameter's draw is a
// column of it.
func TestTheTriangleRebuildsTheCovariance(test *testing.T) {
	rapid.Check(test, rebuildsTheCovariance)
}

// rebuildsTheCovariance is the property on one drawn covariance, for a
// star at the sky's origin, where a shift along the sky is a shift in
// ascension, and where half the draws cross the turn's seam: each is
// held within one turn, and its shift is read across the seam.
func rebuildsTheCovariance(draw *rapid.T) {
	star := theStar()
	star.ra, star.dec = none, none
	covariance := drawnCovariance(draw)
	var sigma moves
	for _, each := range fiveParameters() {
		sigma[each] = math.Sqrt(covariance[each][each])
	}
	star.raError, star.decError = float32(sigma[gaia.RA]),
		float32(sigma[gaia.Dec])
	star.parallaxError = float32(sigma[gaia.Parallax])
	star.pmRAError, star.pmDecError = float32(sigma[gaia.PMRA]),
		float32(sigma[gaia.PMDec])
	for which, pair := range archivePairs() {
		star.corrs[which] = float32(covariance[pair.one][pair.other] /
			(sigma[pair.one] * sigma[pair.other]))
	}
	source := first(draw, ofStars(draw, star))
	made := spread(draw, source)
	var columns [fiveOfThem]moves
	for _, each := range fiveParameters() {
		normal := unitAt(draw, int(each))
		drawn := drawnBy(draw, made, normal)
		ascension := drawn.Place().Ascension().InDegrees()
		if !(ascension >= none && ascension < degreesInTurn) {
			draw.Errorf("a draw is at ascension %g, outside one turn", ascension)
		}
		columns[each] = shifts(source, drawn)
	}
	stored := storedSigmas(source)
	for _, one := range fiveParameters() {
		for _, other := range fiveParameters() {
			between, err := source.Correlations().Between(one, other)
			if err != nil {
				draw.Fatal(err)
			}
			want := stored[one] * stored[other] * between
			var got float64
			for _, column := range columns {
				got += column[one] * column[other]
			}
			if !(math.Abs(got-want) <= tightRoom) {
				draw.Errorf("the triangle gives %g between %d and %d, want %g",
					got, one, other, want)
			}
		}
	}
}

// drawnCovariance is a drawn matrix times its transpose, with the
// margin down its diagonal: positive definite whatever is drawn.
func drawnCovariance(draw *rapid.T) [fiveOfThem]moves {
	var weights, covariance [fiveOfThem]moves
	for row := range weights {
		for col := range weights[row] {
			weights[row][col] = rapid.Float64Range(-anyWeight,
				anyWeight).Draw(draw, "weight")
		}
	}
	for row := range covariance {
		for col := range covariance[row] {
			for which := range weights[row] {
				covariance[row][col] += weights[row][which] * weights[col][which]
			}
		}
		covariance[row][row] += margin
	}
	return covariance
}

// storedSigmas are a source's five errors as it holds them, in
// milliarcseconds and milliarcseconds a year.
func storedSigmas(source *gaia.Source) moves {
	raError, decError := source.PlaceError()
	pmRAError, pmDecError := source.MotionError()
	return moves{masOf(raError), masOf(decError),
		masOf(source.ParallaxError()), pmRAError, pmDecError}
}

// the anti-test: a covariance that is not positive definite is refused
// with nothing beside the error, saying where: three parameters each
// strongly tied to the other two, one of the ties the wrong way; an
// error the archive left empty, which is not a number; and an error of
// nothing, which draws nothing.
func TestACovarianceNoDrawCanComeFromIsRefused(test *testing.T) {
	tied := theStar()
	tied.corrs[raDec], tied.corrs[raParallax] = strong, strong
	tied.corrs[decParallax] = -strong
	empty := theStar()
	empty.parallaxError = float32(math.NaN())
	exact := theStar()
	exact.parallaxError = none
	stars := map[string]madeUp{"tied": tied, "empty": empty, "exact": exact}
	for name, star := range stars {
		source := first(test, ofStars(test, star))
		made, err := source.Spread()
		if err == nil || made != nil || !strings.Contains(err.Error(),
			"not positive definite at parallax") {
			test.Errorf("%s gave %v, %v", name, made, err)
		}
	}
}

// the anti-test: a draw that moves a star past a pole is refused with
// nothing beside the error.
func TestADrawPastAPoleIsRefused(test *testing.T) {
	star := theStar()
	star.dec, star.decError = nearThePole, wideDecError
	source := first(test, ofStars(test, star))
	made, dec := spread(test, source), int(gaia.Dec)
	drawn, err := made.Drawn(unitAt(test, dec))
	if err == nil || drawn != nil || !strings.Contains(err.Error(),
		"past a pole") {
		test.Errorf("a draw past the pole gave %v, %v", drawn, err)
	}
}

// the test and the anti-test: a star at the sky's origin drawn one
// standard deviation west comes out just under a whole turn, not below
// nothing, its ascension's error across the seam.
func TestADrawWestOfTheOriginWrapsIntoOneTurn(test *testing.T) {
	star := theStar()
	star.ra, star.dec = none, none
	source := first(test, ofStars(test, star))
	made, ra := spread(test, source), int(gaia.RA)
	drawn := drawnBy(test, made, deviateAt(test, ra, -unitDeviate))
	ascension := drawn.Place().Ascension().InDegrees()
	west := madeRAError / masInArcsecond / arcsecInDeg
	if !near(ascension, degreesInTurn-west, singleRoom) ||
		!(ascension < degreesInTurn) {
		test.Errorf("a draw west of the origin is at %g degrees, want %g",
			ascension, degreesInTurn-west)
	}
}

// the test: a source as another star is that star's place, parallax,
// motion and radial velocity, with the source's own errors and
// correlations: the made-up star's errors on sgr a*, moving out
// along the frame's x axis.
func TestASourceAsAnotherStarKeepsItsErrors(test *testing.T) {
	source := theSource(test)
	frame, err := galactocentric.Published()
	if err != nil {
		test.Fatal(err)
	}
	centre := measure.Origin()
	moving, err := measure.KilometresPerSecond(madeRadial, none, none)
	if err != nil {
		test.Fatal(err)
	}
	sighting, err := frame.Seen(centre, *moving)
	if err != nil {
		test.Fatal(err)
	}
	as := source.As(*sighting)
	same := as.Place() == sighting.Place() &&
		as.Parallax() == sighting.Parallax() &&
		as.Motion() == sighting.Motion() &&
		as.RadialVelocity() == sighting.Radial()
	kept := as.ID() == source.ID() &&
		as.ParallaxError() == source.ParallaxError() &&
		as.Correlations() == source.Correlations() &&
		as.RadialVelocityError() == source.RadialVelocityError()
	if !same || !kept {
		test.Errorf("the source as another star is %+v", as)
	}
}

// the test and the anti-test: a source's state is the frame's own
// placing of it, place and motion; and a source the frame refuses, with
// no parallax, has no state, with nothing beside the refusal.
func TestASourcesStateIsTheFramesPlacing(test *testing.T) {
	source := theSource(test)
	frame, err := galactocentric.Published()
	if err != nil {
		test.Fatal(err)
	}
	state, err := source.State(*frame)
	if err != nil {
		test.Fatal(err)
	}
	at, moving, err := frame.Of(source.Place(), source.Parallax(),
		source.Motion(), source.RadialVelocity())
	if err != nil {
		test.Fatal(err)
	}
	if state.Place() != *at || state.Numbers() != orbit.NewState(*at,
		*moving).Numbers() {
		test.Errorf("the state is %+v, the frame placed %v %v", state, at,
			moving)
	}
	star := theStar()
	star.parallax = none
	nowhere := first(test, ofStars(test, star))
	if state, err := nowhere.State(*frame); err == nil || state != nil {
		test.Errorf("a source with no parallax gave %v, %v", state, err)
	}
}
