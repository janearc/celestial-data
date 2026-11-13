// Package rows is what the catalogue readers share as they read a row:
// a refusal that names what it refuses and wraps why, and a place on
// the sky from a row's degrees.
package rows

import (
	"fmt"
	"math"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/fits"
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

// oneValue is how many values a row a column read holds. It's defined: a
// reader reads one number from each cell.
const oneValue = 1

// Held is a table's column held to its name and kind, to one value a row
// and to no unit, each refusal naming the file. It refuses a column the
// table lacks, and one of another kind, of many values or with a unit.
func Held(table *fits.Table, file, name string, kind byte) (*fits.Column,
	error) {
	found, err := table.Column(name)
	switch {
	case err != nil:
		return nil, celestialdata.Refuse("the %s has no %s", file, name)
	case found.Kind() != kind:
		return nil, celestialdata.Refuse("%s is of kind %c, not %c",
			name, found.Kind(), kind)
	case found.Count() != oneValue:
		return nil, celestialdata.Refuse(
			"%s holds %d values a row, not one", name,
			found.Count())
	case found.Unit() != "":
		return nil, celestialdata.Refuse(
			"%s is in %q, where the %s writes no unit", name,
			found.Unit(), file)
	}
	return found, nil
}
