package galah

import (
	"slices"

	"github.com/janearc/celestial-data"
	"github.com/janearc/celestial/sky"
)

// Star is one row of the allstar file: a star as GALAH observed it,
// with what its pipeline fit to the star's spectrum.
type Star struct {
	sobject             int64
	twoMASS             string
	gaia                int64
	inGaia              bool
	place               *sky.Equatorial
	snr                 float64
	spectroscopic       Flags
	radial, radialError float64
	teff, teffError     float64
	logg, loggError     float64
	vsini, vsiniError   float64
	iron                Abundance
	lithium             Lithium
	abundances          [elementCount]Abundance
}

// SObjectID is the star's id in the survey, its sobject_id.
func (star Star) SObjectID() int64 { return star.sobject }

// TwoMASS is the star's 2MASS identifier, tmass_id.
func (star Star) TwoMASS() string { return star.twoMASS }

// GaiaID is the star's source in Gaia DR3, gaiadr3_source_id, and
// whether the survey gave one.
func (star Star) GaiaID() (int64, bool) { return star.gaia, star.inGaia }

// Place is where the star is on the sky, from Gaia DR3, or nil for a
// row the survey gave no place.
func (star Star) Place() *sky.Equatorial { return star.place }

// SNR is the signal to noise per pixel of the spectrum from the third
// of HERMES's cameras, snr_px_ccd3: the one the survey's abundance cut
// reads.
func (star Star) SNR() float64 { return star.snr }

// Spectroscopic is the star's flag_sp: the bits the pipeline set on its
// parameters. The survey's cut for parameters is none set.
func (star Star) Spectroscopic() Flags { return star.spectroscopic }

// RadialVelocity is the star's radial velocity in km/s, rv_comp_1: its
// primary's, where the pipeline found a companion too.
func (star Star) RadialVelocity() float64 { return star.radial }

// RadialError is the radial velocity's uncertainty in km/s.
func (star Star) RadialError() float64 { return star.radialError }

// Temperature is the star's effective temperature in kelvin, and its
// uncertainty.
func (star Star) Temperature() (value, uncertainty float64) {
	return star.teff, star.teffError
}

// Gravity is the star's surface gravity as log g in cgs units, adjusted
// by its parallax, and its uncertainty.
func (star Star) Gravity() (value, uncertainty float64) {
	return star.logg, star.loggError
}

// Rotation is the star's broadening as v sin i in km/s, fit with no
// macroturbulence, and its uncertainty.
func (star Star) Rotation() (value, uncertainty float64) {
	return star.vsini, star.vsiniError
}

// Iron is the star's iron, [Fe/H] in dex, with its uncertainty and
// flag. The survey's flags page says flag_fe_h is diagnostic and not to
// be used as it stands.
func (star Star) Iron() Abundance { return star.iron }

// Lithium is the star's lithium as an absolute abundance, A(Li).
func (star Star) Lithium() Lithium { return star.lithium }

// Abundance is an element's abundance against iron, [X/Fe], by its
// symbol as Elements gives it. It refuses an element the file holds
// none of against iron.
func (star Star) Abundance(symbol string) (*Abundance, error) {
	at := slices.Index(elements[:], symbol)
	if at < firstElement {
		return nil, celestialdata.Refuse(
			"the allstar file holds no abundance of %q", symbol)
	}
	abundance := star.abundances[at]
	return &abundance, nil
}

// firstElement is the place of the first element. It's defined: a
// place counted from none.
const firstElement = 0

// Abundance is an abundance as the pipeline fit it, in dex, with its
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

// Flags is the bits the pipeline set on the abundance, its flag_X_fe.
// The survey's cut for abundances is none set, with SNR over 30.
func (abundance Abundance) Flags() Flags { return abundance.flags }

// Lithium is A(Li) in dex, with its uncertainties below and above, or
// the upper limit the survey gives where it found none, and its flag.
type Lithium struct {
	value, below, above, limit float64
	flags                      Flags
	flagged                    bool
}

// Value is A(Li) in dex, a_li: not a number where the survey gives only
// an upper limit.
func (lithium Lithium) Value() float64 { return lithium.value }

// Uncertainty is A(Li)'s uncertainty below and above its value, in dex.
func (lithium Lithium) Uncertainty() (below, above float64) {
	return lithium.below, lithium.above
}

// Limit is the upper limit of A(Li) in dex, a_li_upp_lim, where the
// flag says UpperLimit: not a number elsewhere. In the file every row
// whose flag has that bit has a limit and no value, and every other a
// value and no limit, counted 2026-11-06.
func (lithium Lithium) Limit() float64 { return lithium.limit }

// Flags is flag_a_li and whether the survey gave one: it left 25,364 of
// the file's 917,588 rows' empty, counted 2026-11-06. The survey's
// catalogues page says to use a flag below 2: a value or an upper
// limit with no other bit set.
func (lithium Lithium) Flags() (Flags, bool) {
	return lithium.flags, lithium.flagged
}

// Flags is one of the survey's bitmasks.
type Flags int64

// The masks Has reads by, each defined.
const (
	set  = 1 // defined: a mask of the lowest bit, before its shift
	none = 0 // defined: a mask with no bit set
)

// Has is whether a bit is set, by its number: the flags page gives each
// bit's value, two to the number. A bit past the mask's 64 is never
// set.
func (flags Flags) Has(bit uint) bool { return flags&(set<<bit) != none }

// The bits of flag_sp the search reads, by the flags page.
const (
	EmissionLines     = 0  // defined: value 1, emission lines
	SingleLinedBinary = 2  // defined: value 4, a single-line spectroscopic binary
	DoubleLinedBinary = 3  // defined: value 8, a double-line spectroscopic binary
	TripleStar        = 7  // defined: value 128, a triple star warning
	IronWarn          = 10 // defined: value 1024, a warning on [Fe/H]
	LowSNR            = 11 // defined: value 2048, low S/N
	NotConverged      = 12 // defined: value 4096, the fit did not converge
	Extrapolated      = 13 // defined: value 8192, parameters outside the MARCS grid
	NoResults         = 14 // defined: value 16384, no analysis results
)

// The bits of an element's flag, flag_X_fe, by the flags page. Lithium's
// flag, flag_a_li, sets UpperLimit as they do, as the file shows.
const (
	UpperLimit    = 0 // defined: value 1, the value is an upper limit
	NotDetectable = 1 // defined: value 2, the line is not detectable
	FitFailed     = 2 // defined: value 4, the fit did not converge
)
