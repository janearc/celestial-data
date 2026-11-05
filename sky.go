package celestialdata

import (
	"fmt"
	"math"
)

// The galactic frame's place in ICRS by the Hipparcos definition. It is
// set out in ESA SP-1200 volume 1 section 1.5.3.
const (
	// poleRA is the right ascension of the north galactic pole. It's
	// defined.
	poleRA = 192.85948
	// poleDec is the declination of the north galactic pole. It's
	// defined.
	poleDec = 27.12825
	// poleLon is the galactic longitude of the north celestial pole. It's
	// defined.
	poleLon = 122.93192
)

// halfTurn is half of one turn, in degrees. It's defined.
const halfTurn = 180

// Galactic is a place on the sky in galactic longitude and latitude, in
// degrees.
type Galactic struct {
	lon, lat float64
}

// Longitude is the galactic longitude in degrees, from 0 to under 360.
func (place Galactic) Longitude() float64 { return place.lon }

// Latitude is the galactic latitude in degrees, from -90 to 90.
func (place Galactic) Latitude() float64 { return place.lat }

// Galactic is the place in galactic coordinates.
func (place Position) Galactic() Galactic {
	ra, dec := radians(place.ra), radians(place.dec)
	northRA, northDec := radians(poleRA), radians(poleDec)
	sinDec, cosDec := math.Sincos(dec)
	sinPole, cosPole := math.Sincos(northDec)
	sinRA, cosRA := math.Sincos(ra - northRA)
	up := sinDec*sinPole + cosDec*cosPole*cosRA
	across := cosDec * sinRA
	along := sinDec*cosPole - cosDec*sinPole*cosRA
	turned := degrees(math.Atan2(across, along))
	risen := math.Asin(clamped(up))
	return Galactic{lon: onceRound(poleLon - turned), lat: degrees(risen)}
}

// clamped is a sine kept from -1 to 1. Rounding may push it past them.
func clamped(sine float64) float64 {
	return math.Max(-unit, math.Min(unit, sine))
}

// unit is the largest a sine can be. It's defined.
const unit = 1

// radians is an angle in degrees, in radians.
func radians(angle float64) float64 { return angle * math.Pi / halfTurn }

// degrees is an angle in radians, in degrees.
func degrees(angle float64) float64 { return angle * halfTurn / math.Pi }

// onceRound is a longitude brought from 0 to under 360. Longitudes a hair
// below none round to a full turn on the way. The last step makes that
// none.
func onceRound(longitude float64) float64 {
	within := math.Mod(longitude, raEnd) + raEnd
	return math.Mod(within, raEnd)
}

// apart is the difference of two longitudes taken the short way round.
// It is from -180 to under 180.
func apart(from, to float64) float64 {
	return onceRound(to-from+halfTurn) - halfTurn
}

// Frame is the coordinates a patch of sky is laid out in.
type Frame string

// The frames a patch may be laid out in.
const (
	// InICRS is right ascension and declination.
	InICRS Frame = "icrs"
	// InGalactic is galactic longitude and latitude.
	InGalactic Frame = "galactic"
)

// Patch is a box of sky in a frame. It has a centre and how far it reaches
// either side of it along each axis. The reach is in that frame's degrees.
type Patch struct {
	frame                      Frame
	lon, lat, halfLon, halfLat float64
}

// NewPatch is the box round a centre in a frame. It refuses a frame it does
// not know, a centre off the sky and a reach that is below nothing or not a
// number.
func NewPatch(frame Frame, lon, lat, halfLon, halfLat float64) (*Patch,
	error) {
	if frame != InICRS && frame != InGalactic {
		return nil, fmt.Errorf("%w: frame %q", errRefused, frame)
	}
	if !finite(lon) || !finite(lat) || lat < southPole ||
		lat > northPole {
		return nil, fmt.Errorf("%w: a centre at %v, %v", errRefused,
			lon, lat)
	}
	if !finite(halfLon) || !finite(halfLat) || halfLon < pointlike ||
		halfLat < pointlike {
		return nil, fmt.Errorf("%w: a reach of %v by %v", errRefused,
			halfLon, halfLat)
	}
	return &Patch{frame: frame, lon: onceRound(lon), lat: lat,
		halfLon: halfLon, halfLat: halfLat}, nil
}

// Reaches is whether some of the circle may fall in the patch. It errs
// only by saying yes.
//
// Each degree of longitude is shorter by the cosine of the latitude. So the
// circle's reach in longitude is taken where the circle comes nearest a
// pole. Circles round a pole reach every longitude.
func (circle Near) Reaches(patch *Patch) bool {
	lon, lat := circle.centre.ra, circle.centre.dec
	if patch.frame == InGalactic {
		galactic := circle.centre.Galactic()
		lon, lat = galactic.lon, galactic.lat
	}
	if math.Abs(lat-patch.lat) > patch.halfLat+circle.radius {
		return false
	}
	nearest := math.Abs(lat) + circle.radius
	if nearest >= northPole {
		return true
	}
	shrink := math.Cos(radians(nearest))
	across := math.Abs(apart(patch.lon, lon))
	return across <= patch.halfLon+circle.radius/shrink
}

// pixelEdge is how far the edge of an image is before its first pixel's
// centre, in pixels. It's defined: FITS counts pixel centres from one.
const pixelEdge = 0.5

// Span is where an image's axis is centred and how far it reaches either
// side. It is read from the axis's header cards. value is the coordinate
// at the reference pixel. pixel is that pixel. step is the coordinate's
// change from one pixel to the next. length is the count of pixels.
//
//ninja:vow practice a library: images of the sky are read elsewhere too
func Span(value, pixel, step, length float64) (centre, half float64) {
	first := value + (pixelEdge-pixel)*step
	last := value + (length+pixelEdge-pixel)*step
	return (first + last) / both, math.Abs(last-first) / both
}

// both is the count of a span's ends. It's defined.
const both = 2

// Moved is the same box centred on another longitude, its reach in
// longitude grown by more. Surveys whose tiles are all one size give
// each tile's box from one tile's this way.
//
// More below nothing grows it by nothing. Longitudes that are not a
// number leave a box only circles round a pole reach, as they reach
// every longitude.
func (patch *Patch) Moved(lon, more float64) *Patch {
	moved := *patch
	moved.lon = onceRound(lon)
	moved.halfLon += math.Max(pointlike, more)
	return &moved
}
