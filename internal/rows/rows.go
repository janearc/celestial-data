// Package rows is what the catalogue readers share as they read a row:
// a refusal that names what it refuses and wraps why, and a place on
// the sky from a row's degrees.
package rows

import (
	"fmt"
	"math"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/measure"
	"github.com/janearc/celestial/sky"
)

// Refusing is a refusal of what the format names, wrapping the error
// that says why: errors.Is finds both it and celestialdata's Refused.
func Refusing(why error, format string, args ...any) error {
	what := fmt.Sprintf(format, args...)
	return fmt.Errorf("%w: %s: %w", celestialdata.Refused(), what, why)
}

// Place is a place on the sky by its degrees. It refuses a place past a
// turn or a pole, and a number that is not one.
func Place(ascension, declination float64) (*sky.Equatorial, error) {
	return sky.NewEquatorial(measure.Degrees(ascension),
		measure.Degrees(declination))
}

// Placed is a place on the sky by its degrees, or none when both were
// left empty, as a survey leaves a row it gave no place. It refuses as
// Place does, and one empty number alone.
func Placed(ascension, declination float64) (*sky.Equatorial, error) {
	if math.IsNaN(ascension) && math.IsNaN(declination) {
		return nil, nil
	}
	return Place(ascension, declination)
}
