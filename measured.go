package celestialdata

import (
	"errors"
	"fmt"
	"math"

	"github.com/janearc/puzzlemouth/numbers"
)

// Measured is a numbers file an adapter keeps beside its code, read. Its
// numbers are read from the archive's own files, such as the header cards
// that say where an image lies.
type Measured struct {
	file   string
	read   numbers.Numbers
	unread error
}

// NewMeasured reads a numbers file's bytes. When the file will not read,
// why is kept and each Get gives it.
func NewMeasured(file string, raw []byte) *Measured {
	read, err := numbers.Read(file, raw)
	return &Measured{file: file, read: read, unread: err}
}

// Get is the number of a name, or why there is none.
func (kept *Measured) Get(name string) (float64, error) {
	if kept.unread != nil {
		return math.NaN(), kept.unread
	}
	number, err := kept.read.Get(name)
	if err != nil {
		return math.NaN(), fmt.Errorf("%s: %w", kept.file, err)
	}
	return number.Value, nil
}

// Value is the number of a name. The numbers file is built into the
// adapter beside it, so a name it does not hold, or a file that will not
// read, is a mistake in the adapter's own code, which its tests find:
// asking panics, and says which, as celestial's research.Value does.
func (kept *Measured) Value(name string) float64 {
	number, err := kept.Get(name)
	if err != nil {
		panic(fmt.Errorf("celestial-data: %w", err))
	}
	return number
}

// Footprint is the patch an image covers in a frame. It is read from the
// image's header cards. They are kept as numbers named for the image and
// the card, such as palpha-naxis1.
//
// The first axis is longitude and the second latitude. In ICRS a step
// along the first axis is a step on the sky. So the reach in right
// ascension is wider by one over the cosine of the declination.
func (kept *Measured) Footprint(frame Frame, image string) (*Patch,
	error) {
	lon, halfLon, lonErr := kept.axis(image, "1")
	lat, halfLat, latErr := kept.axis(image, "2")
	if err := errors.Join(lonErr, latErr); err != nil {
		return nil, err
	}
	if frame == InICRS {
		halfLon /= math.Cos(radians(lat))
	}
	return NewPatch(frame, lon, lat, halfLon, halfLat)
}

// axis is the span of one of an image's axes, read from its four cards.
// number is the axis's number as the cards write it.
func (kept *Measured) axis(image, number string) (centre, half float64,
	err error) {
	value, valueErr := kept.Get(image + "-crval" + number)
	pixel, pixelErr := kept.Get(image + "-crpix" + number)
	step, stepErr := kept.Get(image + "-cdelt" + number)
	length, lengthErr := kept.Get(image + "-naxis" + number)
	centre, half = Span(value, pixel, step, length)
	return centre, half, errors.Join(valueErr, pixelErr, stepErr,
		lengthErr)
}

// Circle is the circle three of the numbers name. They are a right
// ascension, a declination and a radius in degrees. Numbers that are
// missing are an error. So is a circle that is not one.
func (kept *Measured) Circle(ra, dec, radius string) (*Near, error) {
	east, eastErr := kept.Get(ra)
	north, northErr := kept.Get(dec)
	reach, reachErr := kept.Get(radius)
	if err := errors.Join(eastErr, northErr, reachErr); err != nil {
		return nil, err
	}
	centre, err := NewPosition(east, north)
	if err != nil {
		return nil, err
	}
	return NewNear(centre, reach)
}
