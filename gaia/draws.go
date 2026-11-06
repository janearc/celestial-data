package gaia

import (
	"math"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/galactocentric"
	"github.com/janearc/celestial/measure"
	"github.com/janearc/celestial/orbit"
	"github.com/janearc/celestial/sky"
)

// Normal is a source of draws of a standard normal, as math/rand's
// Rand is. It is the caller's, so a source's draws repeat when the
// caller's do.
type Normal interface {
	NormFloat64() float64
}

// Spread is a source's errors made ready to draw from: the lower
// triangle whose product with its transpose is the covariance of the
// five astrometric parameters, beside the source itself.
type Spread struct {
	source Source
	lower  [parameters][parameters]float64
}

// Spread is the source's errors ready to draw from, the covariance of
// its five parameters built from their errors and correlations: the
// ascension's error is along the sky, times the cosine of the
// declination, as the archive gives it.
//
// It refuses a covariance that is not positive definite, which no draw
// can come from, with nil beside the error. An error the archive left
// empty is one of those, since it is not a number.
func (source Source) Spread() (*Spread, error) {
	sigma := source.sigmas()
	var lower [parameters][parameters]float64
	for row := range parameters {
		for col := range row + oneMore {
			between := source.correlations.between[row][col]
			products := dotted(lower, row, col)
			sum := sigma[row]*sigma[col]*between - products
			if row != col {
				lower[row][col] = sum / lower[col][col]
				continue
			}
			if !(sum > noSpread) {
				return nil, source.indefinite(row)
			}
			lower[row][col] = math.Sqrt(sum)
		}
	}
	return &Spread{source: source, lower: lower}, nil
}

// dotted is the sum of two rows' products in the triangle, across the
// columns before a column.
func dotted(lower [parameters][parameters]float64, row,
	col Parameter) float64 {
	var sum float64
	for earlier := range col {
		sum += lower[row][earlier] * lower[col][earlier]
	}
	return sum
}

// indefinite is the refusal of a covariance that is not positive
// definite, at the parameter where the triangle found it.
func (source Source) indefinite(row Parameter) error {
	return celestialdata.Refuse("source %d's covariance is not positive "+
		"definite at %s", source.id, parameterNames[row])
}

// sigmas are the five parameters' errors in milliarcseconds and
// milliarcseconds a year, in the archive's order.
func (source Source) sigmas() [parameters]float64 {
	return [parameters]float64{
		source.raError.InArcseconds() * masInArcsecond,
		source.decError.InArcseconds() * masInArcsecond,
		source.parallaxError.InArcseconds() * masInArcsecond,
		source.pmRAError, source.pmDecError}
}

// Drawn is the source moved by its errors, with six draws of the
// normal: one for each of the five parameters in the archive's order,
// through the covariance's triangle so they move together as their
// correlations have them, then one for the radial velocity.
//
// The drawn source keeps the source's errors and correlations.
//
// The drawn ascension is within one turn, as the archive's are. It
// refuses a draw that moves the source past a pole, with nil beside the
// error.
func (spread Spread) Drawn(normal Normal) (*Source, error) {
	var deviates [parameters]float64
	for each := range parameters {
		deviates[each] = normal.NormFloat64()
	}
	radial := normal.NormFloat64()
	var shift [parameters]float64
	for row := range parameters {
		for col := range row + oneMore {
			shift[row] += spread.lower[row][col] * deviates[col]
		}
	}
	drawn := spread.source
	place := drawn.place
	along := mas(shift[RA]).InRadians() /
		math.Cos(place.Declination().InRadians())
	round := place.Ascension().InRadians() + along
	wrapped := math.Mod(round, turn)
	if wrapped < turnStart {
		wrapped += turn
	}
	ascension := measure.Radians(wrapped)
	declination := measure.Radians(place.Declination().InRadians() +
		mas(shift[Dec]).InRadians())
	seen, err := sky.NewEquatorial(ascension, declination)
	if err != nil {
		why := "source %d drawn past a pole: %v"
		return nil, celestialdata.Refuse(why, drawn.id, err)
	}
	drawn.place = *seen
	drawn.parallax = mas(drawn.parallax.InArcseconds()*masInArcsecond +
		shift[Parallax])
	drawn.motion = galactocentric.NewProperMotion(
		drawn.motion.Ascension()+shift[PMRA],
		drawn.motion.Declination()+shift[PMDec])
	drawn.radial += drawn.radialError * radial
	return &drawn, nil
}

// The spread's own numbers, each defined.
const (
	oneMore   = 1 // defined: a row's triangle runs to its own column
	noSpread  = 0 // defined: a variance of nothing, or less, draws nothing
	turnStart = 0 // defined: an ascension of nothing, where a turn starts
)

// turn is a whole turn in radians.
const turn = 2 * math.Pi // defined: a circle is two pi radians round

// parameterNames are the five parameters as the refusals name them.
var parameterNames = [parameters]string{"ra", "dec", "parallax", "pmra",
	"pmdec"}

// As is what the archive would hold for another star, measured as well
// as this one: the sighting's place, parallax, proper motion and radial
// velocity, with this source's id, errors, correlations and the rest.
//
// A star made in the frame borrows a real star's errors so, to be drawn
// as Gaia would have measured it.
func (source Source) As(sighting galactocentric.Sighting) Source {
	moved := source
	moved.place = sighting.Place()
	moved.parallax = sighting.Parallax()
	moved.motion = sighting.Motion()
	moved.radial = sighting.Radial()
	return moved
}

// State is the source's place and motion about the galactic centre, in
// the frame given, as celestial's orbit solver runs it; or the frame's
// refusal, a parallax that gives no distance or a motion past light,
// with nil beside it.
func (source Source) State(frame galactocentric.Frame) (*orbit.State,
	error) {
	at, moving, err := frame.Of(source.place, source.parallax,
		source.motion, source.radial)
	if err != nil {
		return nil, err
	}
	state := orbit.NewState(*at, *moving)
	return &state, nil
}
