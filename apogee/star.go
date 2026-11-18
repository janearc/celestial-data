package apogee

import (
	"slices"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/sky"
)

// Star is one row of the allStar file: a star as APOGEE observed it,
// with what ASPCAP fit to its combined spectrum.
type Star struct {
	gaia                         int64
	id, telescope, field         string
	place                        *sky.Equatorial
	snr                          float64
	radial, scatter, radialError float64
	teff, teffError              float64
	logg, loggError              float64
	metals, metalsError          float64
	alpha, alphaError            float64
	iron                         Abundance
	aspcap, conditions           Flags
	targeting                    Flags
	abundances                   [elementCount]Abundance
}

// GaiaID is the star's source in Gaia, its GAIAEDR3_SOURCE_ID: EDR3's
// source ids are DR3's.
func (star Star) GaiaID() int64 { return star.gaia }

// ID is the star's id in the survey, its APOGEE_ID.
func (star Star) ID() string { return star.id }

// Telescope is the telescope the row's spectra came from.
func (star Star) Telescope() string { return star.telescope }

// Field is the survey's field the row's spectra came from.
func (star Star) Field() string { return star.field }

// Place is where the star is on the sky, or nil for a row the survey
// gave no place, as its first row, VESTA.
func (star Star) Place() *sky.Equatorial { return star.place }

// SNR is the combined spectrum's signal to noise.
func (star Star) SNR() float64 { return star.snr }

// RadialVelocity is the star's barycentric radial velocity in km/s,
// its VHELIO_AVG: the visits' average, weighted by their signal to
// noise.
func (star Star) RadialVelocity() float64 { return star.radial }

// RadialScatter is the scatter of the visits' radial velocities about
// their average in km/s, VSCATTER: a binary's orbit shows here.
func (star Star) RadialScatter() float64 { return star.scatter }

// RadialError is the average radial velocity's uncertainty in km/s,
// VERR.
func (star Star) RadialError() float64 { return star.radialError }

// Temperature is the star's effective temperature in kelvin, and its
// uncertainty.
func (star Star) Temperature() (value, uncertainty float64) {
	return star.teff, star.teffError
}

// Gravity is the star's surface gravity as log g in cgs units, and its
// uncertainty.
func (star Star) Gravity() (value, uncertainty float64) {
	return star.logg, star.loggError
}

// Metals is the star's metallicity, [M/H] in dex, and its uncertainty.
func (star Star) Metals() (value, uncertainty float64) {
	return star.metals, star.metalsError
}

// Alpha is the star's alpha elements against its metals, [alpha/M] in
// dex, and its uncertainty.
func (star Star) Alpha() (value, uncertainty float64) {
	return star.alpha, star.alphaError
}

// Iron is the star's iron, [Fe/H] in dex, with its uncertainty and
// flag.
func (star Star) Iron() Abundance { return star.iron }

// Abundance is an element's abundance against iron, [X/Fe], by its
// symbol as Elements gives it. It refuses an element the file holds
// none of.
func (star Star) Abundance(symbol string) (*Abundance, error) {
	at := slices.Index(elements[:], symbol)
	if at < firstElement {
		return nil, celestialdata.Refuse(
			"the allStar file holds no abundance of %q", symbol)
	}
	abundance := star.abundances[at]
	return &abundance, nil
}

// firstElement is the place of the first element. It's defined: a
// place counted from none.
const firstElement = 0

// ASPCAP is the star's ASPCAPFLAG: the bits ASPCAP set on its fit.
func (star Star) ASPCAP() Flags { return star.aspcap }

// Conditions is the star's STARFLAG: the bits its visits set on their
// spectra.
func (star Star) Conditions() Flags { return star.conditions }

// Targeting is the star's EXTRATARG: why it was observed, and whether
// the row is a duplicate.
func (star Star) Targeting() Flags { return star.targeting }

// Abundance is an abundance as ASPCAP fit it, in dex, with its
// uncertainty and its flag.
type Abundance struct {
	value, uncertainty float64
	flags              Flags
}

// Value is the abundance in dex.
func (abundance Abundance) Value() float64 { return abundance.value }

// Uncertainty is the abundance's uncertainty in dex.
func (abundance Abundance) Uncertainty() float64 {
	return abundance.uncertainty
}

// Flags is the bits ASPCAP set on the abundance, its ELEMFLAG.
func (abundance Abundance) Flags() Flags { return abundance.flags }

// Flags is one of the survey's bitmasks.
type Flags int64

// The masks Has reads by, each defined.
const (
	set  = 1 // defined: a mask of the lowest bit, before its shift
	none = 0 // defined: a mask with no bit set
)

// Has is whether a bit is set, by its number on the survey's bitmask
// page. A bit past the mask's 64 is never set.
func (flags Flags) Has(bit uint) bool { return flags&(set<<bit) != none }

// The bits of ASPCAPFLAG a star's fit is judged by.
const (
	MetalsWarn = 3  // defined: M_H_WARN
	StarWarn   = 7  // defined: STAR_WARN, set with any warning
	MetalsBad  = 19 // defined: M_H_BAD
	StarBad    = 23 // defined: STAR_BAD, set with any bad, a grid edge
	// among them.
	// MultipleSuspect is defined: MULTIPLE_SUSPECT, likely a composite
	// spectrum.
	MultipleSuspect = 41
)

// The bits of an element's flag, ELEMFLAG's.
const (
	// GridEdgeBad is defined: GRIDEDGE_BAD, within an eighth of a grid
	// step of its edge.
	GridEdgeBad = 0
	// CalibrationBad is defined: CALRANGE_BAD, outside the calibrated
	// range.
	CalibrationBad = 1
	// GridEdgeWarn is defined: GRIDEDGE_WARN, within a grid step of
	// its edge.
	GridEdgeWarn    = 8
	CalibrationWarn = 9 // defined: CALRANGE_WARN, where the calibration
	// may not hold.
)

// Duplicate is the bit of EXTRATARG set on a row that is not its
// star's best: another row has a higher signal to noise. It's defined.
const Duplicate = 4
